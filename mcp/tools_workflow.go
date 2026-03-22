package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	batteryclient "github.com/AndroidGoLab/jni-proxy/grpc/client/battery"
	displayclient "github.com/AndroidGoLab/jni-proxy/grpc/client/display"
	locationclient "github.com/AndroidGoLab/jni-proxy/grpc/client/location"
	powerclient "github.com/AndroidGoLab/jni-proxy/grpc/client/power"
	displaypb "github.com/AndroidGoLab/jni-proxy/proto/display"
	handlepb "github.com/AndroidGoLab/jni-proxy/proto/handlestore"
	locationpb "github.com/AndroidGoLab/jni-proxy/proto/location"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func boolPtr(b bool) *bool { return &b }

// jsonResult marshals v as indented JSON and wraps it in a CallToolResult.
func jsonResult(v any) (*gomcp.CallToolResult, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal result: %w", err)
	}
	return &gomcp.CallToolResult{
		Content: []gomcp.Content{&gomcp.TextContent{Text: string(data)}},
	}, nil
}

func (s *Server) registerWorkflowTools() {
	s.registerBatteryTools()
	s.registerLocationTools()
	s.registerDisplayTools()
}

// Android BatteryManager property constants.
const (
	batteryPropertyChargeCounter  int32 = 1
	batteryPropertyCurrentNow     int32 = 2
	batteryPropertyCurrentAverage int32 = 3
	batteryPropertyCapacity       int32 = 4
	batteryPropertyEnergyCounter  int32 = 5
	batteryPropertyStatus         int32 = 6
)

type batteryInput struct{}

type batteryOutput struct {
	Capacity       int32 `json:"capacity"`
	Status         int32 `json:"status"`
	CurrentNowUA   int32 `json:"current_now_ua"`
	CurrentAvgUA   int32 `json:"current_avg_ua"`
	ChargeCounter  int32 `json:"charge_counter_uah"`
	EnergyCounter  int32 `json:"energy_counter_nwh"`
	IsCharging     bool  `json:"is_charging"`
	IsInteractive  bool  `json:"is_interactive"`
	IsPowerSave    bool  `json:"is_power_save_mode"`
	IsDeviceIdle   bool  `json:"is_device_idle_mode"`
	ChargeTimeLeft int64 `json:"charge_time_remaining_ms"`
}

func (s *Server) registerBatteryTools() {
	gomcp.AddTool(s.mcp, &gomcp.Tool{
		Name:        "get_battery_status",
		Description: "Get battery status: capacity %, charging state, current draw, power save mode, and related power information.",
		Annotations: &gomcp.ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			Title:           "Get Battery Status",
		},
	}, func(ctx context.Context, req *gomcp.CallToolRequest, _ batteryInput) (*gomcp.CallToolResult, batteryOutput, error) {
		bat := batteryclient.NewClient(s.conn)
		pwr := powerclient.NewClient(s.conn)

		var out batteryOutput
		var err error

		out.Capacity, err = bat.GetIntProperty(ctx, batteryPropertyCapacity)
		if err != nil {
			return nil, out, fmt.Errorf("get capacity: %w", err)
		}

		out.Status, err = bat.GetIntProperty(ctx, batteryPropertyStatus)
		if err != nil {
			return nil, out, fmt.Errorf("get status: %w", err)
		}

		out.CurrentNowUA, err = bat.GetIntProperty(ctx, batteryPropertyCurrentNow)
		if err != nil {
			return nil, out, fmt.Errorf("get current_now: %w", err)
		}

		out.CurrentAvgUA, err = bat.GetIntProperty(ctx, batteryPropertyCurrentAverage)
		if err != nil {
			return nil, out, fmt.Errorf("get current_avg: %w", err)
		}

		out.ChargeCounter, err = bat.GetIntProperty(ctx, batteryPropertyChargeCounter)
		if err != nil {
			return nil, out, fmt.Errorf("get charge_counter: %w", err)
		}

		out.EnergyCounter, err = bat.GetIntProperty(ctx, batteryPropertyEnergyCounter)
		if err != nil {
			return nil, out, fmt.Errorf("get energy_counter: %w", err)
		}

		out.IsCharging, err = bat.IsCharging(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("is_charging: %w", err)
		}

		out.ChargeTimeLeft, err = bat.ComputeChargeTimeRemaining(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("charge_time_remaining: %w", err)
		}

		out.IsInteractive, err = pwr.IsInteractive(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("is_interactive: %w", err)
		}

		out.IsPowerSave, err = pwr.IsPowerSaveMode(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("is_power_save: %w", err)
		}

		out.IsDeviceIdle, err = pwr.IsDeviceIdleMode(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("is_device_idle: %w", err)
		}

		return nil, out, nil
	})
}

type locationInput struct {
	Provider string `json:"provider" jsonschema:"default=gps,description=Location provider: gps or network"`
}

type locationOutput struct {
	Provider  string  `json:"provider"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Altitude  float64 `json:"altitude"`
	Accuracy  float32 `json:"accuracy"`
	Speed     float32 `json:"speed"`
	Bearing   float32 `json:"bearing"`
	Time      int64   `json:"time"`
}

func (s *Server) registerLocationTools() {
	gomcp.AddTool(s.mcp, &gomcp.Tool{
		Name:        "get_location",
		Description: "Get last known GPS or network location: latitude, longitude, altitude, accuracy, speed, bearing, and timestamp.",
		Annotations: &gomcp.ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			Title:           "Get Location",
		},
	}, func(ctx context.Context, req *gomcp.CallToolRequest, in locationInput) (*gomcp.CallToolResult, locationOutput, error) {
		provider := in.Provider
		if provider == "" {
			provider = "gps"
		}

		locMgr := locationclient.NewClient(s.conn)
		handle, err := locMgr.GetLastKnownLocation(ctx, provider)
		if err != nil {
			return nil, locationOutput{}, fmt.Errorf("get last known location: %w", err)
		}
		if handle == 0 {
			return nil, locationOutput{}, fmt.Errorf("no location available for provider %q (null handle)", provider)
		}

		// Release the handle when done.
		handles := handlepb.NewHandleStoreServiceClient(s.conn)
		defer func() {
			_, _ = handles.ReleaseHandle(ctx, &handlepb.ReleaseHandleRequest{Handle: handle})
		}()

		// Query location properties via the LocationService.
		// The server maps object-level RPCs to the handle returned above.
		locSvc := locationpb.NewLocationServiceClient(s.conn)

		var out locationOutput
		out.Provider = provider

		latResp, err := locSvc.GetLatitude(ctx, &locationpb.GetLatitudeRequest{})
		if err != nil {
			return nil, out, fmt.Errorf("get latitude: %w", err)
		}
		out.Latitude = latResp.GetResult()

		lngResp, err := locSvc.GetLongitude(ctx, &locationpb.GetLongitudeRequest{})
		if err != nil {
			return nil, out, fmt.Errorf("get longitude: %w", err)
		}
		out.Longitude = lngResp.GetResult()

		altResp, err := locSvc.GetAltitude(ctx, &locationpb.GetAltitudeRequest{})
		if err != nil {
			return nil, out, fmt.Errorf("get altitude: %w", err)
		}
		out.Altitude = altResp.GetResult()

		accResp, err := locSvc.GetAccuracy(ctx, &locationpb.GetAccuracyRequest{})
		if err != nil {
			return nil, out, fmt.Errorf("get accuracy: %w", err)
		}
		out.Accuracy = accResp.GetResult()

		speedResp, err := locSvc.GetSpeed(ctx, &locationpb.GetSpeedRequest{})
		if err != nil {
			return nil, out, fmt.Errorf("get speed: %w", err)
		}
		out.Speed = speedResp.GetResult()

		bearingResp, err := locSvc.GetBearing(ctx, &locationpb.GetBearingRequest{})
		if err != nil {
			return nil, out, fmt.Errorf("get bearing: %w", err)
		}
		out.Bearing = bearingResp.GetResult()

		timeResp, err := locSvc.GetTime(ctx, &locationpb.GetTimeRequest{})
		if err != nil {
			return nil, out, fmt.Errorf("get time: %w", err)
		}
		out.Time = timeResp.GetResult()

		return nil, out, nil
	})
}

type displayInput struct{}

type displayOutput struct {
	Width       int32   `json:"width"`
	Height      int32   `json:"height"`
	Rotation    int32   `json:"rotation"`
	RefreshRate float32 `json:"refresh_rate"`
	State       int32   `json:"state"`
	Name        string  `json:"name"`
}

func (s *Server) registerDisplayTools() {
	gomcp.AddTool(s.mcp, &gomcp.Tool{
		Name:        "get_display_info",
		Description: "Get display information: screen resolution, rotation, refresh rate, state, and name.",
		Annotations: &gomcp.ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			Title:           "Get Display Info",
		},
	}, func(ctx context.Context, req *gomcp.CallToolRequest, _ displayInput) (*gomcp.CallToolResult, displayOutput, error) {
		dispMgr := displayclient.NewClient(s.conn)
		handle, err := dispMgr.GetDefaultDisplay(ctx)
		if err != nil {
			return nil, displayOutput{}, fmt.Errorf("get default display: %w", err)
		}
		if handle == 0 {
			return nil, displayOutput{}, fmt.Errorf("no default display available (null handle)")
		}

		// Release the handle when done.
		handles := handlepb.NewHandleStoreServiceClient(s.conn)
		defer func() {
			_, _ = handles.ReleaseHandle(ctx, &handlepb.ReleaseHandleRequest{Handle: handle})
		}()

		// Query display properties via the DisplayService.
		// The server maps object-level RPCs to the handle returned above.
		dispSvc := displaypb.NewDisplayServiceClient(s.conn)

		var out displayOutput

		wResp, err := dispSvc.GetWidth(ctx, &displaypb.GetWidthRequest{})
		if err != nil {
			return nil, out, fmt.Errorf("get width: %w", err)
		}
		out.Width = wResp.GetResult()

		hResp, err := dispSvc.GetHeight(ctx, &displaypb.GetHeightRequest{})
		if err != nil {
			return nil, out, fmt.Errorf("get height: %w", err)
		}
		out.Height = hResp.GetResult()

		rotResp, err := dispSvc.GetRotation(ctx, &displaypb.GetRotationRequest{})
		if err != nil {
			return nil, out, fmt.Errorf("get rotation: %w", err)
		}
		out.Rotation = rotResp.GetResult()

		rrResp, err := dispSvc.GetRefreshRate(ctx, &displaypb.GetRefreshRateRequest{})
		if err != nil {
			return nil, out, fmt.Errorf("get refresh rate: %w", err)
		}
		out.RefreshRate = rrResp.GetResult()

		stResp, err := dispSvc.GetState(ctx, &displaypb.GetStateRequest{})
		if err != nil {
			return nil, out, fmt.Errorf("get state: %w", err)
		}
		out.State = stResp.GetResult()

		nameResp, err := dispSvc.GetName(ctx, &displaypb.GetNameRequest{})
		if err != nil {
			return nil, out, fmt.Errorf("get name: %w", err)
		}
		out.Name = nameResp.GetResult()

		return nil, out, nil
	})
}
