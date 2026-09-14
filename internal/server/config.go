package server

import (
	"fmt"
	"net"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tidwall/redcon"

	"github.com/elcuervo/emb/internal/config"
)

// configParam is one entry in the runtime-config registry. Read-only params
// (listen, tls_*, models) are reported by CONFIG GET but rejected by CONFIG SET.
type configParam struct {
	name     string
	get      func(s *Server) string
	set      func(s *Server, v string) error
	readOnly bool
}

// configParams declares the runtime-settable surface in a fixed order so
// CONFIG GET output is stable.
func (s *Server) configParams() []configParam {
	return []configParam{
		{
			name: "cache",
			get:  func(s *Server) string { return s.cacheConfig },
			set:  (*Server).setConfigCache,
		},
		{
			name: "cache_file",
			get:  func(s *Server) string { return s.persistenceValue("file") },
			set:  (*Server).setConfigCacheFile,
		},
		{
			name:     "cache_load",
			get:      func(s *Server) string { return s.persistenceValue("load") },
			readOnly: true,
		},
		{
			name: "cache_save",
			get:  func(s *Server) string { return s.persistenceValue("save") },
			set:  (*Server).setConfigCacheSave,
		},
		{
			name: "cache_save_on_shutdown",
			get:  func(s *Server) string { return s.persistenceValue("shutdown") },
			set:  (*Server).setConfigCacheSaveOnShutdown,
		},
		{
			name:     "cache_restore_limit",
			get:      func(s *Server) string { return s.persistenceValue("limit") },
			readOnly: true,
		},
		{
			name:     "cache_restore_reserve",
			get:      func(s *Server) string { return s.persistenceValue("reserve") },
			readOnly: true,
		},
		{
			name: "cache_save_rate_limit",
			get:  func(s *Server) string { return s.persistenceValue("rate") },
			set:  (*Server).setConfigCacheSaveRate,
		},
		{
			name: "max_texts",
			get:  func(s *Server) string { return strconv.Itoa(s.maxTexts) },
			set:  intCapSetter(&s.maxTexts, "max_texts"),
		},
		{
			name: "max_pairs",
			get:  func(s *Server) string { return strconv.Itoa(s.maxPairs) },
			set:  intCapSetter(&s.maxPairs, "max_pairs"),
		},
		{
			name: "max_images",
			get:  func(s *Server) string { return strconv.Itoa(s.maxImages) },
			set:  intCapSetter(&s.maxImages, "max_images"),
		},
		{
			name: "max_image_bytes",
			get:  func(s *Server) string { return strconv.FormatInt(s.maxImageBytes, 10) },
			set:  int64CapSetter(&s.maxImageBytes, "max_image_bytes"),
		},
		{
			name: "max_image_pixels",
			get:  func(s *Server) string { return strconv.FormatInt(s.maxImagePixels, 10) },
			set:  int64CapSetter(&s.maxImagePixels, "max_image_pixels"),
		},
		{
			name: "max_command_bytes",
			get:  func(s *Server) string { return strconv.FormatInt(s.maxCommandBytes, 10) },
			set:  (*Server).setConfigMaxCommandBytes,
		},
		{
			name: "password",
			get:  func(s *Server) string { return s.password.Load().(string) },
			set: func(s *Server, v string) error {
				if s.password.Load().(string) == "" && !s.loopbackListener() {
					return fmt.Errorf("setting a password requires a loopback-only listener when no password is configured")
				}
				s.password.Store(v)
				return nil
			},
		},
		{name: "listen", get: func(s *Server) string { return s.addr }, readOnly: true},
		{name: "tls_cert", get: func(s *Server) string { return s.tlsCert }, readOnly: true},
		{name: "tls_key", get: func(s *Server) string { return s.tlsKey }, readOnly: true},
		{name: "models", get: (*Server).configModels, readOnly: true},
	}
}

func (s *Server) persistenceValue(name string) string {
	s.persistenceMu.RLock()
	defer s.persistenceMu.RUnlock()
	switch name {
	case "file":
		return s.cacheFile
	case "load":
		return strconv.FormatBool(s.cacheLoad)
	case "save":
		return s.cacheSave
	case "shutdown":
		return strconv.FormatBool(s.cacheSaveOnShutdown)
	case "limit":
		if s.cacheRestoreLimit == "" {
			return "auto"
		}
		return s.cacheRestoreLimit
	case "reserve":
		if s.cacheRestoreReserve == "" {
			return "10%"
		}
		return s.cacheRestoreReserve
	case "rate":
		if s.cacheSaveRateLimit == "" {
			return "0"
		}
		return s.cacheSaveRateLimit
	default:
		return ""
	}
}

// loopbackListener reports whether the server binds a loopback address only.
// An empty host (":6379") binds every interface, so it is not loopback;
// explicit "localhost" resolves to loopback and is treated as safe.
func (s *Server) loopbackListener() bool {
	host, _, err := net.SplitHostPort(s.addr)
	if err != nil {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// persistenceControlAllowed gates live snapshot-destination and save commands
// on an operator-configured password or a loopback-only listener. An exposed,
// unauthenticated listener must not be able to point cache_file at arbitrary
// writable paths: EMB.SAVE would then clobber that path with snapshot data.
func (s *Server) persistenceControlAllowed() bool {
	return s.password.Load().(string) != "" || s.loopbackListener()
}

func (s *Server) reconfigureSnapshotLocked() {
	interval := time.Duration(0)
	if s.cacheSave != "" {
		interval, _ = time.ParseDuration(s.cacheSave)
	}
	rate, _ := (config.Config{CacheSaveRateLimit: s.cacheSaveRateLimit}).CacheSaveRateBytes()
	if s.snapshot == nil && s.cacheFile != "" && s.cache != nil {
		s.snapshot = newSnapshotCoordinator(s.cache, s.reg, PersistenceConfig{
			File: s.cacheFile, Load: s.cacheLoad, SaveInterval: interval,
			SaveOnShutdown: s.cacheSaveOnShutdown, SaveRateBytes: rate,
			RestoreLimit: s.cacheRestoreLimit, RestoreReserve: s.cacheRestoreReserve,
			SaveRateRaw: s.cacheSaveRateLimit,
		})
	} else if s.snapshot != nil {
		s.snapshot.Configure(s.cacheFile, interval, s.cacheSaveOnShutdown, rate)
	}
}

func (s *Server) setConfigCacheFile(v string) error {
	if !s.persistenceControlAllowed() {
		return fmt.Errorf("cache_file can only be changed with a configured password or a loopback-only listener")
	}
	s.persistenceMu.Lock()
	s.cacheFile = v
	s.reconfigureSnapshotLocked()
	s.persistenceMu.Unlock()
	return nil
}

func (s *Server) setConfigCacheSave(v string) error {
	var interval time.Duration
	var err error
	if v != "" {
		interval, err = time.ParseDuration(v)
		if err != nil || interval <= 0 {
			return fmt.Errorf("cache_save must be a positive duration")
		}
	}
	s.persistenceMu.Lock()
	if v != "" && s.cacheFile == "" {
		s.persistenceMu.Unlock()
		return fmt.Errorf("cache_save requires cache_file")
	}
	s.cacheSave = v
	s.reconfigureSnapshotLocked()
	s.persistenceMu.Unlock()
	return nil
}

func (s *Server) setConfigCacheSaveOnShutdown(v string) error {
	enabled, err := strconv.ParseBool(v)
	if err != nil {
		return fmt.Errorf("cache_save_on_shutdown must be true or false")
	}
	s.persistenceMu.Lock()
	s.cacheSaveOnShutdown = enabled
	s.reconfigureSnapshotLocked()
	s.persistenceMu.Unlock()
	return nil
}

func (s *Server) setConfigCacheSaveRate(v string) error {
	if _, err := (config.Config{CacheSaveRateLimit: v}).CacheSaveRateBytes(); err != nil {
		return err
	}
	s.persistenceMu.Lock()
	s.cacheSaveRateLimit = v
	s.reconfigureSnapshotLocked()
	s.persistenceMu.Unlock()
	return nil
}

// configModels reports the loaded model names (sorted for stable output).
func (s *Server) configModels() string {
	names := make([]string, 0, len(s.reg.List()))
	for _, m := range s.reg.List() {
		names = append(names, m.Name)
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

// setConfigCache resizes the cache budget live. Reuses parseCacheConfig so
// "auto", "N%", and byte sizes behave exactly like boot-time config. Enabling a
// cache that was disabled at boot is restart-only (nil cache has no budget).
func (s *Server) setConfigCache(v string) error {
	if strings.TrimSpace(v) == "" {
		return fmt.Errorf("cache cannot be disabled at runtime; set a size, \"auto\", or a percentage")
	}
	bytes, err := parseCacheConfig(v)
	if err != nil {
		return err
	}
	if s.cache == nil {
		return fmt.Errorf("cache was disabled at boot; restart with a cache size to configure it at runtime")
	}
	s.cache.SetMaxBytes(bytes)
	return nil
}

// intCapSetter / int64CapSetter build the CONFIG SET handler for a
// non-negative integer cap. One helper covers every max_* key, so the parse
// rule and error wording live in one place; label names the key in errors.
func intCapSetter(target *int, label string) func(*Server, string) error {
	return func(_ *Server, v string) error {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return fmt.Errorf("%s must be a non-negative integer", label)
		}
		*target = n
		return nil
	}
}

func int64CapSetter(target *int64, label string) func(*Server, string) error {
	return func(_ *Server, v string) error {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n < 0 {
			return fmt.Errorf("%s must be a non-negative integer", label)
		}
		*target = n
		return nil
	}
}

// setConfigMaxCommandBytes updates the command-size cap and both pre-buffer
// reader guards (per-bulk and aggregate). Values are propagated to connections
// accepted after the change.
func (s *Server) setConfigMaxCommandBytes(v string) error {
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 {
		return fmt.Errorf("max_command_bytes must be a non-negative integer")
	}
	s.maxCommandBytes = n
	if s.srv != nil {
		s.srv.SetMaxBulkSize(n)
		s.srv.SetMaxCommandSize(n)
	}
	return nil
}

func (s *Server) handleConfig(conn redcon.Conn, cmd redcon.Command) {
	args := cmd.Args[1:]
	if len(args) == 0 {
		conn.WriteError("ERR wrong number of arguments for 'CONFIG' command")
		return
	}

	switch strings.ToUpper(string(args[0])) {
	case "GET":
		pattern := ""
		if len(args) >= 2 {
			pattern = string(args[1])
		}
		params := s.configParams()
		matched := make([]configParam, 0, len(params))
		for _, p := range params {
			if pattern == "" {
				matched = append(matched, p)
				continue
			}
			ok, err := path.Match(pattern, p.name)
			if err == nil && ok {
				matched = append(matched, p)
			}
		}
		writePairs(conn, len(matched))
		for _, p := range matched {
			conn.WriteBulkString(p.name)
			conn.WriteBulkString(p.get(s))
		}

	case "SET":
		if len(args) != 3 {
			conn.WriteError("ERR wrong number of arguments for 'CONFIG SET' command")
			return
		}
		name := string(args[1])
		value := string(args[2])
		for _, p := range s.configParams() {
			if p.name != name {
				continue
			}
			if p.readOnly {
				conn.WriteError(fmt.Sprintf("ERR Unsupported CONFIG parameter: %s (read-only, restart required)", name))
				return
			}
			if err := p.set(s, value); err != nil {
				conn.WriteError(fmt.Sprintf("ERR %v", err))
				return
			}
			conn.WriteString("OK")
			return
		}
		conn.WriteError(fmt.Sprintf("ERR Unsupported CONFIG parameter: %s", name))

	default:
		conn.WriteError(fmt.Sprintf("ERR unknown CONFIG subcommand '%s'", string(args[0])))
	}
}
