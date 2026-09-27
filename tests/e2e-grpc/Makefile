# E2E gRPC tests for jnicli against jniservice on Android.
#
# Usage:
#   make -C tests/e2e-grpc test JNISERVICE_LISTEN=192.168.1.20
#   make -C tests/e2e-grpc test-go JNISERVICE_LISTEN=192.168.1.20
#   make -C tests/e2e-grpc test-sh JNISERVICE_LISTEN=192.168.1.20
#   make -C tests/e2e-grpc deploy JNISERVICE_LISTEN=192.168.1.20

ANDROID_SDK ?= $(HOME)/Android/Sdk
NDK ?= $(shell ls -d $(ANDROID_SDK)/ndk/* 2>/dev/null | sort -V | tail -1)
API_LEVEL ?= 30
ADB ?= $(ANDROID_SDK)/platform-tools/adb
CC_ARM64 := $(NDK)/toolchains/llvm/prebuilt/linux-x86_64/bin/aarch64-linux-android$(API_LEVEL)-clang
CC_X86 := $(NDK)/toolchains/llvm/prebuilt/linux-x86_64/bin/x86_64-linux-android$(API_LEVEL)-clang
ARCH ?= $(shell $(ADB) shell getprop ro.product.cpu.abi 2>/dev/null)
REMOTE_DIR := /data/adb/jniservice
REMOTE_ACL_DB ?= $(REMOTE_DIR)/jniservice/acl.db
PORT ?= 50051

ifeq ($(ARCH),arm64-v8a)
  GOARCH := arm64
  CC := $(CC_ARM64)
else
  GOARCH := amd64
  CC := $(CC_X86)
endif

.PHONY: build build-host build-android build-admin push push-admin deploy \
	start-server stop-server require-listen test test-go test-sh clean

# Build host + android binaries.
build: build-host build-android

# Build jnicli and jniserviceadmin for the host (used by test runners).
build-host:
	@mkdir -p build
	CGO_ENABLED=0 go build -o build/jnicli-host ../../cmd/jnicli/
	CGO_ENABLED=0 go build -o build/jniserviceadmin-host ../../cmd/jniserviceadmin/

# Build jniserviceadmin for Android (for granting permissions on device).
build-admin:
	@mkdir -p build
	CGO_ENABLED=0 GOOS=linux GOARCH=$(GOARCH) go build -o build/jniserviceadmin ../../cmd/jniserviceadmin/

# Build jnicli + libjniservice.so + classes.dex for Android.
build-android:
	@mkdir -p build
	CGO_ENABLED=0 GOOS=linux GOARCH=$(GOARCH) go build -o build/jnicli ../../cmd/jnicli/
	CGO_ENABLED=1 GOOS=android GOARCH=$(GOARCH) CC=$(CC) \
		go build -buildmode=c-shared -o build/libjniservice.so ../../cmd/jniservice/
	javac --release 17 \
		-cp $$(ls -d $(ANDROID_SDK)/platforms/android-* | sort -V | tail -1)/android.jar \
		-d build \
		../../cmd/jniservice/JNIService.java \
		../../cmd/jniservice/apk/src/center/dx/jni/internal/GoAbstractDispatch.java \
		../../cmd/jniservice/apk/src/center/dx/jni/generated/*.java \
		../../internal/testjvm/testdata/center/dx/jni/internal/GoInvocationHandler.java
	$(ANDROID_SDK)/build-tools/$$(ls $(ANDROID_SDK)/build-tools | sort -V | tail -1)/d8 \
		--lib $$(ls -d $(ANDROID_SDK)/platforms/android-* | sort -V | tail -1)/android.jar \
		--output build $$(find build -name '*.class')

# Push artifacts to the device.
push: build-android
	$(ADB) shell "mkdir -p $(REMOTE_DIR)"
	$(ADB) push build/jnicli $(REMOTE_DIR)/jnicli
	$(ADB) shell chmod +x $(REMOTE_DIR)/jnicli
	$(ADB) push build/libjniservice.so $(REMOTE_DIR)/libjniservice.so
	$(ADB) push build/classes.dex $(REMOTE_DIR)/jniservice.dex

# Push jniserviceadmin to the device.
push-admin: build-admin
	$(ADB) shell "mkdir -p $(REMOTE_DIR)"
	$(ADB) push build/jniserviceadmin $(REMOTE_DIR)/jniserviceadmin
	$(ADB) shell chmod +x $(REMOTE_DIR)/jniserviceadmin

# Deploy and start the service on this phone's configured Wi-Fi address.
deploy: require-listen
	$(MAKE) push
	$(MAKE) start-server

# Require a concrete per-phone listener address for host-to-phone LAN access.
require-listen:
	@test -n "$(JNISERVICE_LISTEN)" || { \
		echo "JNISERVICE_LISTEN must be set to this phone's current Wi-Fi IPv4 address"; \
		exit 2; \
	}

# Start the gRPC server on the device (kills any old instance first).
start-server: require-listen
	$(MAKE) stop-server
	$(ADB) shell "JNISERVICE_LISTEN='$(JNISERVICE_LISTEN)' \
		JNISERVICE_PORT='$(PORT)' \
		JNISERVICE_DATA_DIR=$(REMOTE_DIR)/data \
		LD_LIBRARY_PATH=$(REMOTE_DIR) \
		app_process \
		-Djava.class.path=$(REMOTE_DIR)/jniservice.dex \
		$(REMOTE_DIR) JNIService" &
	@echo "Waiting for jniservice to start..."
	@for i in 1 2 3 4 5 6 7 8 9 10; do \
		sleep 1; \
		if $(ADB) shell "netstat -tlnp 2>/dev/null | grep -q ':$(PORT)'" 2>/dev/null; then \
			echo "jniservice is listening on port $(PORT)"; \
			exit 0; \
		fi; \
		echo "  attempt $$i/10..."; \
	done; \
	echo "WARNING: could not confirm server is listening; proceeding anyway"; \
	sleep 2

# Stop the server on the device.
stop-server:
	-@$(ADB) shell "pkill -f JNIService" 2>/dev/null || true
	@sleep 1

# Full E2E test pipeline: build, deploy, test, clean up.
test: require-listen build push push-admin start-server test-go
	@$(MAKE) stop-server

# Run Go E2E tests against the phone's direct-LAN address.
# Uses ADB mode: jniserviceadmin runs on the device via "adb shell"
# to grant permissions in the device-side ACL database.
test-go: build-host require-listen
	JNICTL_E2E_ADDR=$(JNISERVICE_LISTEN):$(PORT) \
	JNICTL_BIN=$$(pwd)/build/jnicli-host \
	JNISERVICEADMIN_ADB_BIN=$(REMOTE_DIR)/jniserviceadmin \
	JNISERVICE_DB=$(REMOTE_ACL_DB) \
	ADB=$(ADB) \
		go test -v -run TestE2E ./

# Run shell-based E2E tests against the phone's direct-LAN address.
test-sh: build-host require-listen
	JNICTL_E2E_ADDR=$(JNISERVICE_LISTEN):$(PORT) JNICTL_BIN=$$(pwd)/build/jnicli-host \
		./run_e2e.sh

clean:
	rm -rf build/
