## 1. Dockerfiles

- [x] 1.1 Create `Dockerfile` with multi-stage build (Go 1.25 + ONNX Runtime 1.21.0 → runtime)
- [x] 1.2 Create `.dockerignore` excluding models/, bin/, .git, and IDE files

## 2. justfile Targets

- [x] 2.1 Add `docker-build` target: builds multi-arch image with buildx
- [x] 2.2 Add `docker-push` target: builds and pushes `elcuervo/emb-server`

## 3. Verification

- [x] 3.1 Run `just docker-build` for native architecture (at least one platform)
- [x] 3.2 Verify `docker run elcuervo/emb-server` starts and responds to PING
- [x] 3.3 Clean up test images
