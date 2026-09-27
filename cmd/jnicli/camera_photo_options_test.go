package main

import (
	"strings"
	"testing"
	"time"
)

// TestParseCameraPhotoFlashPolicy verifies the supported user-facing flash values.
//
// Example: "auto" selects the camera's automatic-flash AE mode.
func TestParseCameraPhotoFlashPolicy(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		want    cameraPhotoFlashPolicy
		wantErr bool
	}{
		{name: "off", value: "off", want: cameraPhotoFlashOff},
		{name: "auto", value: "AUTO", want: cameraPhotoFlashAuto},
		{name: "on with whitespace", value: " on ", want: cameraPhotoFlashOn},
		{name: "invalid", value: "torch", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseCameraPhotoFlashPolicy(test.value)
			if test.wantErr {
				if err == nil {
					t.Fatalf("parseCameraPhotoFlashPolicy(%q) returned nil error", test.value)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseCameraPhotoFlashPolicy(%q): %v", test.value, err)
			}
			if got != test.want {
				t.Fatalf("parseCameraPhotoFlashPolicy(%q) = %d, want %d", test.value, got, test.want)
			}
		})
	}
}

// TestValidateCameraPhotoOptions verifies defaults and incompatible manual controls.
//
// Example: ISO and exposure must be supplied together, and manual capture cannot use flash.
func TestValidateCameraPhotoOptions(t *testing.T) {
	valid := cameraPhotoOptions{
		aeWarmup: time.Second,
		flash:    cameraPhotoFlashOff,
		iso:      400,
		exposure: 10 * time.Millisecond,
	}
	if err := valid.validate(); err != nil {
		t.Fatalf("valid manual options rejected: %v", err)
	}

	defaults := cameraPhotoOptions{
		aeWarmup: 3 * time.Second,
		flash:    cameraPhotoFlashOff,
	}
	if err := defaults.validate(); err != nil {
		t.Fatalf("default auto options rejected: %v", err)
	}

	invalid := []struct {
		name string
		opts cameraPhotoOptions
		want string
	}{
		{
			name: "ISO without exposure",
			opts: cameraPhotoOptions{flash: cameraPhotoFlashOff, iso: 400},
			want: "both --iso and --exposure",
		},
		{
			name: "exposure without ISO",
			opts: cameraPhotoOptions{flash: cameraPhotoFlashOff, exposure: time.Millisecond},
			want: "both --iso and --exposure",
		},
		{
			name: "manual flash",
			opts: cameraPhotoOptions{
				flash:    cameraPhotoFlashAuto,
				iso:      400,
				exposure: 10 * time.Millisecond,
			},
			want: "manual ISO/exposure require --flash off",
		},
		{
			name: "negative warmup",
			opts: cameraPhotoOptions{aeWarmup: -time.Second, flash: cameraPhotoFlashOff},
			want: "AE warm-up",
		},
		{
			name: "negative ISO",
			opts: cameraPhotoOptions{
				flash:    cameraPhotoFlashOff,
				iso:      -1,
				exposure: time.Millisecond,
			},
			want: "ISO",
		},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			err := test.opts.validate()
			if err == nil {
				t.Fatal("validate returned nil error")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validate error %q does not contain %q", err, test.want)
			}
		})
	}
}
