package main

import (
	"fmt"
	"os"

	pb "github.com/AndroidGoLab/jni-proxy/proto/jni_raw"
	"github.com/spf13/cobra"
)

var cameraPhotoCmd = &cobra.Command{
	Use:   "photo",
	Short: "Take a JPEG photo",
	Long: `Take a JPEG photo using Camera2.

By default this uses auto-exposure, no flash, and waits up to 3s for AE to
converge. Set both --iso and --exposure to use manual exposure; manual mode
disables AE and requires --flash=off.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx, cancel := requestContext(cmd)
		defer cancel()

		cameraIndex, err := cmd.Flags().GetInt("index")
		if err != nil {
			return fmt.Errorf("reading camera index: %w", err)
		}
		if cameraIndex < 0 {
			return fmt.Errorf("camera index must not be negative")
		}
		output, err := cmd.Flags().GetString("output")
		if err != nil {
			return fmt.Errorf("reading output path: %w", err)
		}
		aeWarmup, err := cmd.Flags().GetDuration("ae-timeout")
		if err != nil {
			return fmt.Errorf("reading AE warm-up timeout: %w", err)
		}
		flashName, err := cmd.Flags().GetString("flash")
		if err != nil {
			return fmt.Errorf("reading flash policy: %w", err)
		}
		flash, err := parseCameraPhotoFlashPolicy(flashName)
		if err != nil {
			return err
		}
		iso, err := cmd.Flags().GetInt("iso")
		if err != nil {
			return fmt.Errorf("reading manual ISO: %w", err)
		}
		exposure, err := cmd.Flags().GetDuration("exposure")
		if err != nil {
			return fmt.Errorf("reading manual exposure time: %w", err)
		}
		captureOptions := cameraPhotoOptions{
			aeWarmup: aeWarmup,
			flash:    flash,
			iso:      iso,
			exposure: exposure,
		}
		if err := captureOptions.validate(); err != nil {
			return err
		}

		client := pb.NewJNIServiceClient(grpcConn)

		cls, err := client.FindClass(ctx, &pb.FindClassRequest{
			Name: "center/dx/jni/jniservice/CameraCapture",
		})
		if err != nil {
			return fmt.Errorf("finding CameraCapture class (is the APK installed?): %w", err)
		}

		method, err := client.GetStaticMethodID(ctx, &pb.GetStaticMethodIDRequest{
			ClassHandle: cls.GetClassHandle(),
			Name:        "takePicture",
			Sig:         "(Landroid/content/Context;IIIIJ)[B",
		})
		if err != nil {
			return fmt.Errorf("getting takePicture method: %w", err)
		}

		appCtx, err := client.GetAppContext(ctx, &pb.GetAppContextRequest{})
		if err != nil {
			return fmt.Errorf("getting app context: %w", err)
		}
		contextHandle := appCtx.GetContextHandle()
		result, err := client.CallStaticMethod(ctx, &pb.CallStaticMethodRequest{
			ClassHandle: cls.GetClassHandle(),
			MethodId:    method.GetMethodId(),
			ReturnType:  pb.JType_OBJECT,
			Args: []*pb.JValue{
				{Value: &pb.JValue_L{L: contextHandle}},
				{Value: &pb.JValue_I{I: int32(cameraIndex)}},
				{Value: &pb.JValue_I{I: int32(captureOptions.aeWarmup.Milliseconds())}},
				{Value: &pb.JValue_I{I: int32(captureOptions.flash)}},
				{Value: &pb.JValue_I{I: int32(captureOptions.iso)}},
				{Value: &pb.JValue_J{J: captureOptions.exposure.Nanoseconds()}},
			},
		})
		if err != nil {
			return fmt.Errorf("camera capture failed: %w", err)
		}

		arrayHandle := result.GetResult().GetL()
		if arrayHandle == 0 {
			return fmt.Errorf("camera returned null (check camera permission)")
		}

		data, err := client.GetByteArrayData(ctx, &pb.GetByteArrayDataRequest{
			ArrayHandle: arrayHandle,
		})
		if err != nil {
			return fmt.Errorf("reading image data: %w", err)
		}

		if output == "-" || output == "" {
			_, err := os.Stdout.Write(data.GetData())
			return err
		}
		if err := os.WriteFile(output, data.GetData(), 0644); err != nil {
			return fmt.Errorf("writing %s: %w", output, err)
		}
		fmt.Fprintf(os.Stderr, "Saved %d bytes to %s\n", len(data.GetData()), output)
		return nil
	},
}

func init() {
	cameraPhotoCmd.Flags().Int("index", 0, "camera index (0=back, 1=front)")
	cameraPhotoCmd.Flags().StringP("output", "o", "", "output file (default: stdout)")
	cameraPhotoCmd.Flags().Duration("ae-timeout", cameraPhotoDefaultAEWarmup, "maximum AE warm-up time (0 disables warm-up; max 30s)")
	cameraPhotoCmd.Flags().String("flash", "off", "flash policy: off, auto, or on")
	cameraPhotoCmd.Flags().Int("iso", 0, "manual sensor ISO (requires --exposure; 0 uses AE)")
	cameraPhotoCmd.Flags().Duration("exposure", 0, "manual sensor exposure time (for example, 10ms; requires --iso)")
	cameraCmd.AddCommand(cameraPhotoCmd)
}
