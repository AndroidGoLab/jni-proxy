# jni-proxy

gRPC proxy layer for [AndroidGoLab/jni](https://github.com/AndroidGoLab/jni).

Exposes Android JNI bindings over gRPC so that a host machine can control an
Android device remotely.

## Components

- **cmd/jnicli** — CLI client that talks to jniservice over gRPC
- **cmd/jniservice** — gRPC server that runs on the Android device (APK or Magisk module)
- **cmd/jniserviceadmin** — Admin CLI for ACL management
- **grpc/** — gRPC server and client wrappers
- **proto/** — Protobuf service definitions
- **handlestore/** — Object handle mapping for cross-process JNI references
- **tools/cmd/callbackgen** — Code generator for Java callback adapter classes

## Quick start

```bash
# Build the CLI for Linux
make dist-jnicli-linux

# Build and deploy to a connected Android device
make deploy

# Run E2E tests
make test-emulator
```

## Dependencies

This module depends on `github.com/AndroidGoLab/jni` for core JNI bindings.
When developing locally, use a `go.work` file pointing to both repos.
