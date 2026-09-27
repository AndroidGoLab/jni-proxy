package main

import (
	"fmt"
	"strings"
	"time"
)

// cameraPhotoFlashPolicy identifies the Android AE flash policy for a still capture.
//
// Example: cameraPhotoFlashOff requests auto-exposure without firing the flash.
type cameraPhotoFlashPolicy int32

const (
	// cameraPhotoFlashOff requests auto-exposure without flash.
	cameraPhotoFlashOff cameraPhotoFlashPolicy = iota
	// cameraPhotoFlashAuto lets auto-exposure decide whether to fire the flash.
	cameraPhotoFlashAuto
	// cameraPhotoFlashOn requests auto-exposure with the flash enabled.
	cameraPhotoFlashOn
)

const (
	// cameraPhotoDefaultAEWarmup gives automatic exposure three seconds to converge.
	cameraPhotoDefaultAEWarmup = 3 * time.Second
	// cameraPhotoMaxAEWarmup bounds a single camera request while retaining a practical RPC timeout.
	cameraPhotoMaxAEWarmup = 30 * time.Second
	// cameraPhotoMaxISO is the largest ISO value representable by the Java int API.
	cameraPhotoMaxISO = int64(1<<31 - 1)
)

// cameraPhotoOptions contains the controls passed to the on-device camera helper.
//
// Example: zero ISO and exposure select automatic exposure.
type cameraPhotoOptions struct {
	aeWarmup time.Duration
	flash    cameraPhotoFlashPolicy
	iso      int
	exposure time.Duration
}

// parseCameraPhotoFlashPolicy converts the CLI spelling to the Camera2 policy value.
//
// Example: parseCameraPhotoFlashPolicy("auto") selects automatic flash.
func parseCameraPhotoFlashPolicy(value string) (cameraPhotoFlashPolicy, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "off":
		return cameraPhotoFlashOff, nil
	case "auto":
		return cameraPhotoFlashAuto, nil
	case "on":
		return cameraPhotoFlashOn, nil
	default:
		return 0, fmt.Errorf("invalid flash policy %q: want off, auto, or on", value)
	}
}

// validate checks ranges and the manual-control/AE policy invariant.
//
// Example: manual ISO and exposure are accepted only with flash off.
func (options cameraPhotoOptions) validate() error {
	switch {
	case options.aeWarmup < 0:
		return fmt.Errorf("AE warm-up timeout must not be negative")
	case options.aeWarmup > cameraPhotoMaxAEWarmup:
		return fmt.Errorf("AE warm-up timeout exceeds the maximum of %s", cameraPhotoMaxAEWarmup)
	case options.flash < cameraPhotoFlashOff || options.flash > cameraPhotoFlashOn:
		return fmt.Errorf("invalid flash policy value %d", options.flash)
	case options.iso < 0:
		return fmt.Errorf("ISO must not be negative")
	case int64(options.iso) > cameraPhotoMaxISO:
		return fmt.Errorf("ISO exceeds the Java API limit of %d", cameraPhotoMaxISO)
	case options.exposure < 0:
		return fmt.Errorf("exposure time must not be negative")
	case (options.iso == 0) != (options.exposure == 0):
		return fmt.Errorf("both --iso and --exposure must be set for manual exposure")
	case options.iso != 0 && options.flash != cameraPhotoFlashOff:
		return fmt.Errorf("manual ISO/exposure require --flash off (manual exposure disables AE)")
	default:
		return nil
	}
}
