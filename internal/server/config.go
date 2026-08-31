package server

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/tidwall/redcon"
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
			get:  func(s *Server) string { return s.cacheFile },
			set: func(s *Server, v string) error {
				s.cacheFile = v
				return nil
			},
		},
		{
			name: "cache_save",
			get:  func(s *Server) string { return s.cacheSave },
			set: func(s *Server, v string) error {
				s.cacheSave = v
				return nil
			},
		},
		{
			name: "password",
			get:  func(s *Server) string { return s.password.Load().(string) },
			set: func(s *Server, v string) error {
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
		conn.WriteArray(len(matched) * 2)
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
