.PHONY: generate callbacks proto protoc grpc cli clean lint test test-tools test-e2e test-emulator \
	build build-server list-commands dist dist-jnicli-linux dist-jnicli-android dist-jnimcp-linux dist-jnimcp-android dist-jniserviceadmin dist-jniservice dist-dex \
	magisk apk deploy push start-server stop-server forward

# Path to the jni repo checkout (sibling directory by default).
JNI_DIR ?= ../jni

# Android SDK platform JAR for specgen.
ANDROID_JAR ?= $(ANDROID_HOME)/platforms/android-36/android.jar

# Run all generators
generate: callbacks proto protoc grpc cli

# Run callbackgen — generates Java callback adapter classes
callbacks:
	go run ./tools/cmd/callbackgen/

# Run protogen — generates .proto files from Java API specs
proto:
	go run ./tools/cmd/protogen/ -specs $(JNI_DIR)/spec/java/ -overlays $(JNI_DIR)/spec/overlays/java/ -output proto/ -go-module github.com/AndroidGoLab/jni-proxy
	@mkdir -p proto/handlestore
	@cp $(JNI_DIR)/spec/handlestore.proto proto/handlestore/handlestore.proto

# Run protoc — compiles .proto files to Go stubs
protoc: proto
	@command -v protoc >/dev/null 2>&1 || { echo "protoc not found. Install: https://grpc.io/docs/protoc-installation/"; exit 1; }
	@for dir in proto/*/; do \
		pkg=$$(basename "$$dir"); \
		protoc -I. --go_out=. --go_opt=paths=source_relative \
			--go-grpc_out=. --go-grpc_opt=paths=source_relative \
			"$$dir$$pkg.proto"; \
	done

# Run grpcgen — generates gRPC server and client wrappers
grpc: protoc
	go run ./tools/cmd/grpcgen/ -specs $(JNI_DIR)/spec/java/ -overlays $(JNI_DIR)/spec/overlays/java/ -output . -go-module github.com/AndroidGoLab/jni-proxy -jni-module github.com/AndroidGoLab/jni

# Run cligen — generates jnicli cobra commands from Java API specs
cli: grpc
	go run ./tools/cmd/cligen/ -specs $(JNI_DIR)/spec/java/ -overlays $(JNI_DIR)/spec/overlays/java/ -output cmd/jnicli/ -go-module github.com/AndroidGoLab/jni-proxy

# Run only tool tests (no JDK needed)
test-tools:
	go test ./tools/...

# List all jnicli leaf commands as full paths
list-commands:
	@go run ./cmd/jnicli/ list-commands

# Remove build artifacts
clean:
	rm -rf build/

# Run golangci-lint
lint:
	golangci-lint run ./...

# Cross-compile libraries for android/arm64 (requires NDK toolchain)
build:
	CGO_ENABLED=1 \
	GOOS=android \
	GOARCH=arm64 \
	CC=$(shell echo $$ANDROID_NDK_HOME/toolchains/llvm/prebuilt/linux-x86_64/bin/aarch64-linux-android21-clang) \
	go build ./...

# Cross-compile jniservice as c-shared library for Android
build-server:
	@mkdir -p build
	CGO_ENABLED=1 \
	GOOS=android \
	GOARCH=arm64 \
	CC=$(shell echo $$ANDROID_NDK_HOME/toolchains/llvm/prebuilt/linux-x86_64/bin/aarch64-linux-android21-clang) \
	go build -buildmode=c-shared -o build/libjniservice.so ./cmd/jniservice/

# Android SDK (used by deploy + dist targets).
ifeq ($(ANDROID_SDK),)
  ifneq ($(ANDROID_HOME),)
    ANDROID_SDK := $(ANDROID_HOME)
  else ifneq ($(ANDROID_SDK_ROOT),)
    ANDROID_SDK := $(ANDROID_SDK_ROOT)
  else
    ANDROID_SDK := $(HOME)/Android/Sdk
  endif
endif

# ---- Device deployment ----
ADB ?= $(ANDROID_SDK)/platform-tools/adb
PORT ?= 50051
HOST_PORT ?= $(PORT)
REMOTE_DIR := /data/adb/jniservice

deploy: push start-server forward

push:
	@ARCH=$${DEVICE_ARCH:-$$($(ADB) shell getprop ro.product.cpu.abi)}; \
	case "$$ARCH" in \
		arm64-v8a) GOARCH=arm64; ABI=arm64-v8a ;; \
		*)         GOARCH=amd64; ABI=x86_64 ;; \
	esac; \
	echo "Device arch: $$ARCH -> GOARCH=$$GOARCH ABI=$$ABI"; \
	$(MAKE) dist-jniservice dist-dex DIST_GOARCH=$$GOARCH && \
	$(ADB) shell "mkdir -p $(REMOTE_DIR)" && \
	$(ADB) push build/libjniservice-$$ABI.so $(REMOTE_DIR)/libjniservice.so && \
	$(ADB) push build/classes.dex $(REMOTE_DIR)/jniservice.dex

start-server: stop-server
	$(ADB) shell "JNISERVICE_PORT=$(PORT) \
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

stop-server:
	-@$(ADB) shell "pkill -f JNIService" 2>/dev/null || true
	@sleep 1

forward:
	$(ADB) forward tcp:$(HOST_PORT) tcp:$(PORT)
	@echo "Port forwarding: localhost:$(HOST_PORT) -> device:$(PORT)"

# ---- Tests ----

test-e2e:
	go test ./tests/e2e-grpc/ -v

test-emulator:
	$(MAKE) -C tests/e2e-grpc test

# ---- Release artifacts ----
DIST_GOARCH ?= arm64
DIST_NDK ?= $(shell ls -d $(ANDROID_SDK)/ndk/* 2>/dev/null | sort -V | tail -1)
DIST_API_LEVEL ?= 30

ifeq ($(DIST_GOARCH),arm64)
  DIST_NDK_TRIPLE := aarch64-linux-android
  DIST_ANDROID_ABI := arm64-v8a
else
  DIST_NDK_TRIPLE := x86_64-linux-android
  DIST_ANDROID_ABI := x86_64
endif

DIST_CC := $(DIST_NDK)/toolchains/llvm/prebuilt/linux-x86_64/bin/$(DIST_NDK_TRIPLE)$(DIST_API_LEVEL)-clang

dist: dist-jnicli-linux dist-jnicli-android dist-jnimcp-linux dist-jnimcp-android dist-jniservice dist-jniserviceadmin dist-dex

dist-jnicli-linux:
	@mkdir -p build
	CGO_ENABLED=0 GOOS=linux GOARCH=$(DIST_GOARCH) \
		go build -o build/jnicli-linux-$(DIST_GOARCH) ./cmd/jnicli/

dist-jnimcp-linux:
	@mkdir -p build
	CGO_ENABLED=0 GOOS=linux GOARCH=$(DIST_GOARCH) \
		go build -o build/jnimcp-linux-$(DIST_GOARCH) ./cmd/jnimcp/

dist-jnimcp-android:
	@mkdir -p build
	CGO_ENABLED=1 GOOS=android GOARCH=$(DIST_GOARCH) CC=$(DIST_CC) \
		go build -o build/jnimcp-android-$(DIST_GOARCH) ./cmd/jnimcp/

dist-jnicli-android:
	@mkdir -p build
	CGO_ENABLED=1 GOOS=android GOARCH=$(DIST_GOARCH) CC=$(DIST_CC) \
		go build -o build/jnicli-android-$(DIST_GOARCH) ./cmd/jnicli/

dist-jniserviceadmin:
	@mkdir -p build
	CGO_ENABLED=1 GOOS=android GOARCH=$(DIST_GOARCH) CC=$(DIST_CC) \
		go build -o build/jniserviceadmin-android-$(DIST_GOARCH) ./cmd/jniserviceadmin/

dist-jniservice:
	@mkdir -p build
	CGO_ENABLED=1 GOOS=android GOARCH=$(DIST_GOARCH) CC=$(DIST_CC) \
		go build -buildmode=c-shared \
			-o build/libjniservice-$(DIST_ANDROID_ABI).so ./cmd/jniservice/

dist-dex:
	@mkdir -p build
	javac --release 17 -d build cmd/jniservice/JNIService.java
	$(ANDROID_SDK)/build-tools/$$(ls $(ANDROID_SDK)/build-tools | sort -V | tail -1)/d8 \
		--lib $$(ls -d $(ANDROID_SDK)/platforms/android-* | sort -V | tail -1)/android.jar \
		--output build build/JNIService.class
	rm -f build/JNIService.class

# ---- Magisk module ----
magisk: dist-jniservice dist-dex
	@rm -rf build/magisk-staging
	@mkdir -p build/magisk-staging/jniservice
	@mkdir -p build/magisk-staging/META-INF/com/google/android
	cp cmd/jniservice/magisk/module.prop build/magisk-staging/
	cp cmd/jniservice/magisk/service.sh build/magisk-staging/
	cp cmd/jniservice/magisk/customize.sh build/magisk-staging/
	cp build/libjniservice-$(DIST_ANDROID_ABI).so build/magisk-staging/jniservice/libjniservice.so
	cp build/classes.dex build/magisk-staging/jniservice/jniservice.dex
	@printf '#!/sbin/sh\n. "$$MODPATH/customize.sh"\n' > build/magisk-staging/META-INF/com/google/android/update-binary
	@chmod 755 build/magisk-staging/META-INF/com/google/android/update-binary
	@touch build/magisk-staging/META-INF/com/google/android/updater-script
	cd build/magisk-staging && zip -r ../jniservice-magisk-$(DIST_ANDROID_ABI).zip . -x '*.DS_Store'
	@echo "Built: build/jniservice-magisk-$(DIST_ANDROID_ABI).zip"

# ---- APK ----
APK_SRC := cmd/jniservice/apk
APK_STAGING := build/apk-staging
APK_BUILD_TOOLS ?= $(shell ls -d $(ANDROID_SDK)/build-tools/* 2>/dev/null | sort -V | tail -1)
APK_PLATFORM ?= $(shell ls -d $(ANDROID_SDK)/platforms/android-* 2>/dev/null | sort -V | tail -1)

apk: dist-jniservice
	@rm -rf $(APK_STAGING)
	@mkdir -p $(APK_STAGING)/lib/$(DIST_ANDROID_ABI) $(APK_STAGING)/classes
	cp build/libjniservice-$(DIST_ANDROID_ABI).so $(APK_STAGING)/lib/$(DIST_ANDROID_ABI)/libjniservice.so
	$(APK_BUILD_TOOLS)/aapt2 link \
		-I $(APK_PLATFORM)/android.jar \
		--manifest $(APK_SRC)/AndroidManifest.xml \
		-o $(APK_STAGING)/base.apk \
		--auto-add-overlay
	javac --release 17 \
		-cp $(APK_PLATFORM)/android.jar \
		-d $(APK_STAGING)/classes \
		$(APK_SRC)/src/center/dx/jni/jniservice/*.java \
		$(APK_SRC)/src/center/dx/jni/internal/GoAbstractDispatch.java \
		$(APK_SRC)/src/center/dx/jni/generated/*.java \
		internal/testjvm/testdata/center/dx/jni/internal/GoInvocationHandler.java
	$(APK_BUILD_TOOLS)/d8 \
		--lib $(APK_PLATFORM)/android.jar \
		--output $(APK_STAGING) \
		$$(find $(APK_STAGING)/classes -name '*.class')
	cd $(APK_STAGING) && zip -u base.apk classes.dex
	cd $(APK_STAGING) && zip -u -r base.apk lib/
	$(APK_BUILD_TOOLS)/zipalign -f 4 $(APK_STAGING)/base.apk $(APK_STAGING)/aligned.apk
	@if [ -n "$$JNISERVICE_KEYSTORE" ]; then \
		echo "Signing with provided keystore"; \
		$(APK_BUILD_TOOLS)/apksigner sign \
			--ks "$$JNISERVICE_KEYSTORE" \
			--ks-pass "pass:$$JNISERVICE_KEYSTORE_PASSWORD" \
			--out build/jniservice-$(DIST_ANDROID_ABI).apk \
			$(APK_STAGING)/aligned.apk; \
	else \
		echo "Signing with debug keystore"; \
		if [ ! -f build/debug.keystore ]; then \
			keytool -genkeypair -v -keystore build/debug.keystore \
				-storepass android -keypass android \
				-alias androiddebugkey -keyalg RSA -keysize 2048 \
				-validity 10000 -dname "CN=Debug"; \
		fi; \
		$(APK_BUILD_TOOLS)/apksigner sign \
			--ks build/debug.keystore \
			--ks-pass pass:android \
			--out build/jniservice-$(DIST_ANDROID_ABI).apk \
			$(APK_STAGING)/aligned.apk; \
	fi
	@echo "Built: build/jniservice-$(DIST_ANDROID_ABI).apk"
