package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	alarmclient "github.com/AndroidGoLab/jni-proxy/grpc/client/alarm"
	audioclient "github.com/AndroidGoLab/jni-proxy/grpc/client/audiomanager"
	batteryclient "github.com/AndroidGoLab/jni-proxy/grpc/client/battery"
	cameraclient "github.com/AndroidGoLab/jni-proxy/grpc/client/camera"
	clipboardclient "github.com/AndroidGoLab/jni-proxy/grpc/client/clipboard"
	displayclient "github.com/AndroidGoLab/jni-proxy/grpc/client/display"
	inputmethodclient "github.com/AndroidGoLab/jni-proxy/grpc/client/inputmethod"
	irclient "github.com/AndroidGoLab/jni-proxy/grpc/client/ir"
	jobclient "github.com/AndroidGoLab/jni-proxy/grpc/client/job"
	locationclient "github.com/AndroidGoLab/jni-proxy/grpc/client/location"
	netclient "github.com/AndroidGoLab/jni-proxy/grpc/client/net"
	notifclient "github.com/AndroidGoLab/jni-proxy/grpc/client/notification"
	powerclient "github.com/AndroidGoLab/jni-proxy/grpc/client/power"
	telecomclient "github.com/AndroidGoLab/jni-proxy/grpc/client/telecom"
	telephonyclient "github.com/AndroidGoLab/jni-proxy/grpc/client/telephony"
	vibratorclient "github.com/AndroidGoLab/jni-proxy/grpc/client/vibrator"
	wificlient "github.com/AndroidGoLab/jni-proxy/grpc/client/wifi"
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
	s.registerNetworkTools()
	s.registerWifiTools()
	s.registerTelephonyTools()
	s.registerAudioTools()
	s.registerClipboardTools()
	s.registerNotificationTools()
	s.registerVibratorTools()
	s.registerIRTools()
	s.registerCameraTools()
	s.registerSchedulingTools()
	s.registerTelecomTools()
	s.registerInputTools()
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

// ---------------------------------------------------------------------------
// Network tools (ConnectivityManager)
// ---------------------------------------------------------------------------

type networkInput struct{}

type networkOutput struct {
	ActiveNetworkHandle      int64 `json:"active_network_handle"`
	IsActiveNetworkMetered   bool  `json:"is_active_network_metered"`
	IsDefaultNetworkActive   bool  `json:"is_default_network_active"`
	BackgroundDataAllowed    bool  `json:"background_data_allowed"`
	RestrictBackgroundStatus int32 `json:"restrict_background_status"`
	NetworkPreference        int32 `json:"network_preference"`
}

func (s *Server) registerNetworkTools() {
	// query_network — read-only network status
	gomcp.AddTool(s.mcp, &gomcp.Tool{
		Name:        "query_network",
		Description: "Get active network info: handle, metered status, default-network activity, background data setting, and restrict-background status.",
		Annotations: &gomcp.ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			Title:           "Query Network",
		},
	}, func(ctx context.Context, req *gomcp.CallToolRequest, _ networkInput) (*gomcp.CallToolResult, networkOutput, error) {
		client := netclient.NewClient(s.conn)

		var out networkOutput
		var err error

		out.ActiveNetworkHandle, err = client.GetActiveNetwork(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("get active network: %w", err)
		}

		out.IsActiveNetworkMetered, err = client.IsActiveNetworkMetered(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("is active network metered: %w", err)
		}

		out.IsDefaultNetworkActive, err = client.IsDefaultNetworkActive(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("is default network active: %w", err)
		}

		out.BackgroundDataAllowed, err = client.GetBackgroundDataSetting(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("get background data setting: %w", err)
		}

		out.RestrictBackgroundStatus, err = client.GetRestrictBackgroundStatus(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("get restrict background status: %w", err)
		}

		out.NetworkPreference, err = client.GetNetworkPreference(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("get network preference: %w", err)
		}

		return nil, out, nil
	})
}

// ---------------------------------------------------------------------------
// WiFi tools (WifiManager)
// ---------------------------------------------------------------------------

type scanWifiInput struct{}

type scanWifiOutput struct {
	ScanStarted       bool  `json:"scan_started"`
	ScanResultsHandle int64 `json:"scan_results_handle"`
	WifiState         int32 `json:"wifi_state"`
	IsWifiEnabled     bool  `json:"is_wifi_enabled"`
}

type connectWifiInput struct {
	NetworkID int32 `json:"network_id" jsonschema:"description=Network ID to enable and connect to (from configured networks)"`
}

type setWifiEnabledInput struct {
	Enabled bool `json:"enabled" jsonschema:"description=true to enable WiFi or false to disable"`
}

type setWifiEnabledOutput struct {
	Success       bool  `json:"success"`
	NewWifiState  int32 `json:"new_wifi_state"`
	IsWifiEnabled bool  `json:"is_wifi_enabled"`
}

func (s *Server) registerWifiTools() {
	// scan_wifi — trigger scan and return results handle
	gomcp.AddTool(s.mcp, &gomcp.Tool{
		Name:        "scan_wifi",
		Description: "Trigger a WiFi scan and return the scan results handle, current WiFi state, and whether WiFi is enabled. The scan_results_handle is an opaque Java object handle for the List<ScanResult>.",
		Annotations: &gomcp.ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			Title:           "Scan WiFi Networks",
		},
	}, func(ctx context.Context, req *gomcp.CallToolRequest, _ scanWifiInput) (*gomcp.CallToolResult, scanWifiOutput, error) {
		client := wificlient.NewClient(s.conn)

		var out scanWifiOutput
		var err error

		out.IsWifiEnabled, err = client.IsWifiEnabled(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("is wifi enabled: %w", err)
		}

		out.WifiState, err = client.GetWifiState(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("get wifi state: %w", err)
		}

		out.ScanStarted, err = client.StartScan(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("start scan: %w", err)
		}

		out.ScanResultsHandle, err = client.GetScanResults(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("get scan results: %w", err)
		}

		return nil, out, nil
	})

	// connect_wifi — enable a configured network
	gomcp.AddTool(s.mcp, &gomcp.Tool{
		Name:        "connect_wifi",
		Description: "Connect to a WiFi network by enabling the given network ID (from configured networks) and triggering reconnect. Requires a valid network_id from the device's configured network list.",
		Annotations: &gomcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			Title:           "Connect to WiFi Network",
		},
	}, func(ctx context.Context, req *gomcp.CallToolRequest, in connectWifiInput) (*gomcp.CallToolResult, any, error) {
		client := wificlient.NewClient(s.conn)

		enabled, err := client.EnableNetwork(ctx, in.NetworkID, true)
		if err != nil {
			return nil, nil, fmt.Errorf("enable network %d: %w", in.NetworkID, err)
		}

		reconnected, err := client.Reconnect(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("reconnect: %w", err)
		}

		result := map[string]any{
			"network_id":     in.NetworkID,
			"enable_success": enabled,
			"reconnected":    reconnected,
		}
		r, err := jsonResult(result)
		return r, nil, err
	})

	// set_wifi_enabled — toggle WiFi on/off
	gomcp.AddTool(s.mcp, &gomcp.Tool{
		Name:        "set_wifi_enabled",
		Description: "Enable or disable WiFi. Returns the success status and new WiFi state.",
		Annotations: &gomcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			Title:           "Set WiFi Enabled",
		},
	}, func(ctx context.Context, req *gomcp.CallToolRequest, in setWifiEnabledInput) (*gomcp.CallToolResult, setWifiEnabledOutput, error) {
		client := wificlient.NewClient(s.conn)

		var out setWifiEnabledOutput
		var err error

		ok, err := client.SetWifiEnabled(ctx, in.Enabled)
		if err != nil {
			return nil, out, fmt.Errorf("set wifi enabled: %w", err)
		}
		out.Success = ok

		out.NewWifiState, err = client.GetWifiState(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("get wifi state: %w", err)
		}

		out.IsWifiEnabled, err = client.IsWifiEnabled(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("is wifi enabled: %w", err)
		}

		return nil, out, nil
	})

	// discover_services — stub (callback-based)
	type discoverServicesInput struct{}
	gomcp.AddTool(s.mcp, &gomcp.Tool{
		Name:        "discover_services",
		Description: "Discover mDNS/Bonjour services on the local network (NSD). Currently returns a stub because NSD discovery requires callback streaming.",
		Annotations: &gomcp.ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			Title:           "Discover Network Services",
		},
	}, func(ctx context.Context, req *gomcp.CallToolRequest, _ discoverServicesInput) (*gomcp.CallToolResult, any, error) {
		return &gomcp.CallToolResult{
			Content: []gomcp.Content{&gomcp.TextContent{
				Text: "discover_services requires callback streaming which is not yet supported. " +
					"Use call_android_api or jni_raw for advanced NSD operations.",
			}},
		}, nil, nil
	})

	// wifi_direct — stub (callback-based)
	type wifiDirectInput struct{}
	gomcp.AddTool(s.mcp, &gomcp.Tool{
		Name:        "wifi_direct",
		Description: "WiFi P2P (WiFi Direct) operations: peer discovery, connect, create group. Currently returns a stub because P2P requires callback streaming.",
		Annotations: &gomcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			Title:           "WiFi Direct",
		},
	}, func(ctx context.Context, req *gomcp.CallToolRequest, _ wifiDirectInput) (*gomcp.CallToolResult, any, error) {
		return &gomcp.CallToolResult{
			Content: []gomcp.Content{&gomcp.TextContent{
				Text: "wifi_direct requires callback streaming which is not yet supported. " +
					"Use call_android_api or jni_raw for advanced WiFi P2P operations.",
			}},
		}, nil, nil
	})

	// wifi_rtt_range — stub (callback-based)
	type wifiRttInput struct{}
	gomcp.AddTool(s.mcp, &gomcp.Tool{
		Name:        "wifi_rtt_range",
		Description: "WiFi RTT (Round Trip Time) ranging for indoor positioning. Currently returns a stub because RTT ranging requires callback streaming.",
		Annotations: &gomcp.ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			Title:           "WiFi RTT Ranging",
		},
	}, func(ctx context.Context, req *gomcp.CallToolRequest, _ wifiRttInput) (*gomcp.CallToolResult, any, error) {
		return &gomcp.CallToolResult{
			Content: []gomcp.Content{&gomcp.TextContent{
				Text: "wifi_rtt_range requires callback streaming which is not yet supported. " +
					"Use call_android_api or jni_raw for advanced WiFi RTT operations.",
			}},
		}, nil, nil
	})
}

// ---------------------------------------------------------------------------
// Telephony tools (TelephonyManager)
// ---------------------------------------------------------------------------

type cellularInput struct{}

type cellularOutput struct {
	NetworkOperator     string `json:"network_operator"`
	NetworkOperatorName string `json:"network_operator_name"`
	NetworkCountryISO   string `json:"network_country_iso"`
	SimOperator         string `json:"sim_operator"`
	SimOperatorName     string `json:"sim_operator_name"`
	SimCountryISO       string `json:"sim_country_iso"`
	SimState            int32  `json:"sim_state"`
	PhoneType           int32  `json:"phone_type"`
	NetworkType         int32  `json:"network_type"`
	DataNetworkType     int32  `json:"data_network_type"`
	DataState           int32  `json:"data_state"`
	DataActivity        int32  `json:"data_activity"`
	CallState           int32  `json:"call_state"`
	IsNetworkRoaming    bool   `json:"is_network_roaming"`
	IsDataEnabled       bool   `json:"is_data_enabled"`
	IsDataRoamingEnabled bool  `json:"is_data_roaming_enabled"`
	HasIccCard          bool   `json:"has_icc_card"`
	PhoneCount          int32  `json:"phone_count"`
}

func (s *Server) registerTelephonyTools() {
	// get_cellular_info — read-only telephony/cellular info
	gomcp.AddTool(s.mcp, &gomcp.Tool{
		Name:        "get_cellular_info",
		Description: "Get cellular/telephony info: carrier, signal, SIM state, roaming, data state, call state, network type, and phone count.",
		Annotations: &gomcp.ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			Title:           "Get Cellular Info",
		},
	}, func(ctx context.Context, req *gomcp.CallToolRequest, _ cellularInput) (*gomcp.CallToolResult, cellularOutput, error) {
		client := telephonyclient.NewClient(s.conn)

		var out cellularOutput
		var err error

		out.NetworkOperator, err = client.GetNetworkOperator(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("get network operator: %w", err)
		}

		out.NetworkOperatorName, err = client.GetNetworkOperatorName(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("get network operator name: %w", err)
		}

		out.NetworkCountryISO, err = client.GetNetworkCountryIso0(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("get network country iso: %w", err)
		}

		out.SimOperator, err = client.GetSimOperator(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("get sim operator: %w", err)
		}

		out.SimOperatorName, err = client.GetSimOperatorName(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("get sim operator name: %w", err)
		}

		out.SimCountryISO, err = client.GetSimCountryIso(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("get sim country iso: %w", err)
		}

		out.SimState, err = client.GetSimState0(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("get sim state: %w", err)
		}

		out.PhoneType, err = client.GetPhoneType(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("get phone type: %w", err)
		}

		out.NetworkType, err = client.GetNetworkType(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("get network type: %w", err)
		}

		out.DataNetworkType, err = client.GetDataNetworkType(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("get data network type: %w", err)
		}

		out.DataState, err = client.GetDataState(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("get data state: %w", err)
		}

		out.DataActivity, err = client.GetDataActivity(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("get data activity: %w", err)
		}

		out.CallState, err = client.GetCallState(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("get call state: %w", err)
		}

		out.IsNetworkRoaming, err = client.IsNetworkRoaming(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("is network roaming: %w", err)
		}

		out.IsDataEnabled, err = client.IsDataEnabled(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("is data enabled: %w", err)
		}

		out.IsDataRoamingEnabled, err = client.IsDataRoamingEnabled(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("is data roaming enabled: %w", err)
		}

		out.HasIccCard, err = client.HasIccCard(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("has icc card: %w", err)
		}

		out.PhoneCount, err = client.GetPhoneCount(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("get phone count: %w", err)
		}

		return nil, out, nil
	})
}

// ---------------------------------------------------------------------------
// Audio tools (AudioManager)
// ---------------------------------------------------------------------------

// Android AudioManager stream type constants.
const (
	streamVoiceCall    int32 = 0
	streamSystem       int32 = 1
	streamRing         int32 = 2
	streamMusic        int32 = 3
	streamAlarm        int32 = 4
	streamNotification int32 = 5
)

type audioInput struct{}

type streamVolumeInfo struct {
	Stream int32 `json:"stream"`
	Volume int32 `json:"volume"`
	Min    int32 `json:"min"`
	Max    int32 `json:"max"`
	Muted  bool  `json:"muted"`
}

type audioOutput struct {
	RingerMode      int32              `json:"ringer_mode"`
	Mode            int32              `json:"mode"`
	IsMicMuted      bool               `json:"is_mic_muted"`
	IsMusicActive   bool               `json:"is_music_active"`
	IsSpeakerOn     bool               `json:"is_speakerphone_on"`
	IsBluetoothA2DP bool               `json:"is_bluetooth_a2dp_on"`
	IsBluetoothSCO  bool               `json:"is_bluetooth_sco_on"`
	Streams         []streamVolumeInfo `json:"streams"`
}

type setVolumeInput struct {
	Stream int32 `json:"stream" jsonschema:"description=Audio stream type: 0=voice_call 1=system 2=ring 3=music 4=alarm 5=notification"`
	Volume int32 `json:"volume" jsonschema:"description=Volume level to set (0 to stream max)"`
	Flags  int32 `json:"flags" jsonschema:"default=0,description=Flags: 0=none 1=show_ui 2=allow_ringer_modes 4=play_sound 8=remove_sound_and_vibrate 16=vibrate"`
}

type setRingerModeInput struct {
	Mode int32 `json:"mode" jsonschema:"description=Ringer mode: 0=silent 1=vibrate 2=normal"`
}

func (s *Server) registerAudioTools() {
	// get_audio_status — read-only audio overview
	gomcp.AddTool(s.mcp, &gomcp.Tool{
		Name:        "get_audio_status",
		Description: "Get audio status: volume levels for all streams (voice, system, ring, music, alarm, notification), ringer mode, audio mode, mute states, speakerphone, and Bluetooth audio state.",
		Annotations: &gomcp.ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			Title:           "Get Audio Status",
		},
	}, func(ctx context.Context, req *gomcp.CallToolRequest, _ audioInput) (*gomcp.CallToolResult, audioOutput, error) {
		client := audioclient.NewClient(s.conn)

		var out audioOutput
		var err error

		out.RingerMode, err = client.GetRingerMode(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("get ringer mode: %w", err)
		}

		out.Mode, err = client.GetMode(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("get mode: %w", err)
		}

		out.IsMicMuted, err = client.IsMicrophoneMute(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("is mic muted: %w", err)
		}

		out.IsMusicActive, err = client.IsMusicActive(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("is music active: %w", err)
		}

		out.IsSpeakerOn, err = client.IsSpeakerphoneOn(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("is speakerphone on: %w", err)
		}

		out.IsBluetoothA2DP, err = client.IsBluetoothA2DpOn(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("is bluetooth a2dp on: %w", err)
		}

		out.IsBluetoothSCO, err = client.IsBluetoothScoOn(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("is bluetooth sco on: %w", err)
		}

		// Collect volume info for the standard streams.
		streams := []int32{
			streamVoiceCall, streamSystem, streamRing,
			streamMusic, streamAlarm, streamNotification,
		}
		for _, st := range streams {
			var si streamVolumeInfo
			si.Stream = st

			si.Volume, err = client.GetStreamVolume(ctx, st)
			if err != nil {
				return nil, out, fmt.Errorf("get stream %d volume: %w", st, err)
			}

			si.Min, err = client.GetStreamMinVolume(ctx, st)
			if err != nil {
				return nil, out, fmt.Errorf("get stream %d min: %w", st, err)
			}

			si.Max, err = client.GetStreamMaxVolume(ctx, st)
			if err != nil {
				return nil, out, fmt.Errorf("get stream %d max: %w", st, err)
			}

			si.Muted, err = client.IsStreamMute(ctx, st)
			if err != nil {
				return nil, out, fmt.Errorf("get stream %d mute: %w", st, err)
			}

			out.Streams = append(out.Streams, si)
		}

		return nil, out, nil
	})

	// set_volume — mutation: set stream volume
	gomcp.AddTool(s.mcp, &gomcp.Tool{
		Name:        "set_volume",
		Description: "Set volume for an audio stream. Stream types: 0=voice_call, 1=system, 2=ring, 3=music, 4=alarm, 5=notification. Flags: 0=none, 1=show_ui, 4=play_sound.",
		Annotations: &gomcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			Title:           "Set Volume",
		},
	}, func(ctx context.Context, req *gomcp.CallToolRequest, in setVolumeInput) (*gomcp.CallToolResult, any, error) {
		client := audioclient.NewClient(s.conn)

		err := client.SetStreamVolume(ctx, in.Stream, in.Volume, in.Flags)
		if err != nil {
			return nil, nil, fmt.Errorf("set stream volume: %w", err)
		}

		// Read back the new volume to confirm.
		newVol, err := client.GetStreamVolume(ctx, in.Stream)
		if err != nil {
			return nil, nil, fmt.Errorf("get stream volume after set: %w", err)
		}

		result := map[string]any{
			"stream":     in.Stream,
			"new_volume": newVol,
		}
		r, err := jsonResult(result)
		return r, nil, err
	})

	// set_ringer_mode — mutation: set ringer mode
	gomcp.AddTool(s.mcp, &gomcp.Tool{
		Name:        "set_ringer_mode",
		Description: "Set ringer mode: 0=silent, 1=vibrate, 2=normal.",
		Annotations: &gomcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			Title:           "Set Ringer Mode",
		},
	}, func(ctx context.Context, req *gomcp.CallToolRequest, in setRingerModeInput) (*gomcp.CallToolResult, any, error) {
		client := audioclient.NewClient(s.conn)

		err := client.SetRingerMode(ctx, in.Mode)
		if err != nil {
			return nil, nil, fmt.Errorf("set ringer mode: %w", err)
		}

		// Read back the new ringer mode to confirm.
		newMode, err := client.GetRingerMode(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("get ringer mode after set: %w", err)
		}

		result := map[string]any{
			"new_ringer_mode": newMode,
		}
		r, err := jsonResult(result)
		return r, nil, err
	})
}

// ---------------------------------------------------------------------------
// Clipboard tools (ClipboardManager)
// ---------------------------------------------------------------------------

type clipboardInput struct{}

type clipboardOutput struct {
	HasClip    bool  `json:"has_clip"`
	HasText    bool  `json:"has_text"`
	TextHandle int64 `json:"text_handle,omitempty"`
}

type setClipboardInput struct {
	Text string `json:"text" jsonschema:"description=Text to copy to the clipboard"`
}

func (s *Server) registerClipboardTools() {
	// get_clipboard — read-only clipboard status
	gomcp.AddTool(s.mcp, &gomcp.Tool{
		Name:        "get_clipboard",
		Description: "Get clipboard status: whether a clip is present, whether it contains text, and a handle to the text content. The text_handle is a Java CharSequence object handle that can be inspected via jni_raw.",
		Annotations: &gomcp.ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			Title:           "Get Clipboard",
		},
	}, func(ctx context.Context, req *gomcp.CallToolRequest, _ clipboardInput) (*gomcp.CallToolResult, clipboardOutput, error) {
		client := clipboardclient.NewClient(s.conn)

		var out clipboardOutput
		var err error

		out.HasClip, err = client.HasPrimaryClip(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("has primary clip: %w", err)
		}

		out.HasText, err = client.HasText(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("has text: %w", err)
		}

		if out.HasText {
			out.TextHandle, err = client.GetText(ctx)
			if err != nil {
				return nil, out, fmt.Errorf("get text: %w", err)
			}
		}

		return nil, out, nil
	})

	// set_clipboard — mutation: write text to clipboard
	gomcp.AddTool(s.mcp, &gomcp.Tool{
		Name:        "set_clipboard",
		Description: "Write text to the clipboard. Uses the deprecated ClipboardManager.setText which accepts a plain string directly.",
		Annotations: &gomcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			Title:           "Set Clipboard",
		},
	}, func(ctx context.Context, req *gomcp.CallToolRequest, in setClipboardInput) (*gomcp.CallToolResult, any, error) {
		client := clipboardclient.NewClient(s.conn)

		err := client.SetText(ctx, in.Text)
		if err != nil {
			return nil, nil, fmt.Errorf("set text: %w", err)
		}

		result := map[string]any{
			"success": true,
			"length":  len(in.Text),
		}
		r, err := jsonResult(result)
		return r, nil, err
	})
}

// ---------------------------------------------------------------------------
// Notification tools (NotificationManager)
// ---------------------------------------------------------------------------

type cancelNotifInput struct {
	ID int32 `json:"id" jsonschema:"description=Notification ID to cancel"`
}

type notifStatusInput struct{}

type notifStatusOutput struct {
	NotificationsEnabled bool  `json:"notifications_enabled"`
	NotificationsPaused  bool  `json:"notifications_paused"`
	BubblesAllowed       bool  `json:"bubbles_allowed"`
	BubblesEnabled       bool  `json:"bubbles_enabled"`
	Importance           int32 `json:"importance"`
	InterruptionFilter   int32 `json:"interruption_filter"`
	ChannelsHandle       int64 `json:"channels_handle,omitempty"`
}

func (s *Server) registerNotificationTools() {
	// send_notification — stub (requires Java object construction)
	type sendNotifInput struct {
		Title   string `json:"title" jsonschema:"description=Notification title"`
		Text    string `json:"text" jsonschema:"description=Notification body text"`
		Channel string `json:"channel" jsonschema:"description=Notification channel ID"`
	}
	gomcp.AddTool(s.mcp, &gomcp.Tool{
		Name:        "send_notification",
		Description: "Post a notification to the device. Currently stubbed because building a Notification object requires constructing Java objects via JNI.",
		Annotations: &gomcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			Title:           "Send Notification",
		},
	}, func(ctx context.Context, req *gomcp.CallToolRequest, _ sendNotifInput) (*gomcp.CallToolResult, any, error) {
		return &gomcp.CallToolResult{
			Content: []gomcp.Content{&gomcp.TextContent{
				Text: "send_notification requires constructing a Notification Java object (Notification.Builder). " +
					"Use the jni_raw tool to create the Notification.Builder, set title/text/channel, build it, " +
					"then call NotificationManager.notify(id, notification) via call_android_api.",
			}},
		}, nil, nil
	})

	// cancel_notification — mutation: cancel by ID
	gomcp.AddTool(s.mcp, &gomcp.Tool{
		Name:        "cancel_notification",
		Description: "Cancel a previously posted notification by its ID.",
		Annotations: &gomcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			Title:           "Cancel Notification",
		},
	}, func(ctx context.Context, req *gomcp.CallToolRequest, in cancelNotifInput) (*gomcp.CallToolResult, any, error) {
		client := notifclient.NewClient(s.conn)

		err := client.Cancel1(ctx, in.ID)
		if err != nil {
			return nil, nil, fmt.Errorf("cancel notification %d: %w", in.ID, err)
		}

		result := map[string]any{
			"cancelled_id": in.ID,
		}
		r, err := jsonResult(result)
		return r, nil, err
	})

	// list_notification_channels — read-only notification status and channels
	gomcp.AddTool(s.mcp, &gomcp.Tool{
		Name:        "list_notification_channels",
		Description: "List notification channels and get notification status: enabled, paused, bubbles, importance, interruption filter. The channels_handle is a Java List<NotificationChannel> object handle.",
		Annotations: &gomcp.ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			Title:           "List Notification Channels",
		},
	}, func(ctx context.Context, req *gomcp.CallToolRequest, _ notifStatusInput) (*gomcp.CallToolResult, notifStatusOutput, error) {
		client := notifclient.NewClient(s.conn)

		var out notifStatusOutput
		var err error

		out.NotificationsEnabled, err = client.AreNotificationsEnabled(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("are notifications enabled: %w", err)
		}

		out.NotificationsPaused, err = client.AreNotificationsPaused(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("are notifications paused: %w", err)
		}

		out.BubblesAllowed, err = client.AreBubblesAllowed(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("are bubbles allowed: %w", err)
		}

		out.BubblesEnabled, err = client.AreBubblesEnabled(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("are bubbles enabled: %w", err)
		}

		out.Importance, err = client.GetImportance(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("get importance: %w", err)
		}

		out.InterruptionFilter, err = client.GetCurrentInterruptionFilter(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("get interruption filter: %w", err)
		}

		out.ChannelsHandle, err = client.GetNotificationChannels(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("get notification channels: %w", err)
		}

		return nil, out, nil
	})
}

// ---------------------------------------------------------------------------
// Vibrator tools
// ---------------------------------------------------------------------------

type vibrateInput struct {
	DurationMS int64 `json:"duration_ms" jsonschema:"description=Vibration duration in milliseconds"`
}

type vibratorStatusInput struct{}

type vibratorStatusOutput struct {
	HasVibrator         bool    `json:"has_vibrator"`
	HasAmplitudeControl bool    `json:"has_amplitude_control"`
	VibratorID          int32   `json:"vibrator_id"`
	QFactor             float32 `json:"q_factor,omitempty"`
	ResonantFrequency   float32 `json:"resonant_frequency,omitempty"`
}

func (s *Server) registerVibratorTools() {
	// vibrate — mutation: vibrate for a duration
	gomcp.AddTool(s.mcp, &gomcp.Tool{
		Name:        "vibrate",
		Description: "Vibrate the device for the specified duration in milliseconds.",
		Annotations: &gomcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			Title:           "Vibrate",
		},
	}, func(ctx context.Context, req *gomcp.CallToolRequest, in vibrateInput) (*gomcp.CallToolResult, any, error) {
		client := vibratorclient.NewClient(s.conn)

		err := client.Vibrate1(ctx, in.DurationMS)
		if err != nil {
			return nil, nil, fmt.Errorf("vibrate: %w", err)
		}

		result := map[string]any{
			"vibrating":   true,
			"duration_ms": in.DurationMS,
		}
		r, err := jsonResult(result)
		return r, nil, err
	})

	// get_vibrator_info — read-only vibrator capabilities
	gomcp.AddTool(s.mcp, &gomcp.Tool{
		Name:        "get_vibrator_info",
		Description: "Get vibrator info: whether the device has a vibrator, amplitude control support, vibrator ID, Q factor, and resonant frequency.",
		Annotations: &gomcp.ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			Title:           "Get Vibrator Info",
		},
	}, func(ctx context.Context, req *gomcp.CallToolRequest, _ vibratorStatusInput) (*gomcp.CallToolResult, vibratorStatusOutput, error) {
		client := vibratorclient.NewClient(s.conn)

		var out vibratorStatusOutput
		var err error

		out.HasVibrator, err = client.HasVibrator(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("has vibrator: %w", err)
		}

		out.HasAmplitudeControl, err = client.HasAmplitudeControl(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("has amplitude control: %w", err)
		}

		out.VibratorID, err = client.GetId(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("get id: %w", err)
		}

		// Q factor and resonant frequency may not be available on all devices;
		// treat errors as non-fatal and leave at zero value.
		out.QFactor, _ = client.GetQFactor(ctx)
		out.ResonantFrequency, _ = client.GetResonantFrequency(ctx)

		return nil, out, nil
	})
}

// ---------------------------------------------------------------------------
// IR tools (ConsumerIrManager)
// ---------------------------------------------------------------------------

type irStatusInput struct{}

type irStatusOutput struct {
	HasIrEmitter      bool  `json:"has_ir_emitter"`
	CarrierFreqHandle int64 `json:"carrier_frequencies_handle,omitempty"`
}

func (s *Server) registerIRTools() {
	// ir_transmit — stub (requires int[] handle for pattern)
	type irTransmitInput struct {
		Frequency int32   `json:"frequency" jsonschema:"description=IR carrier frequency in Hz"`
		Pattern   []int32 `json:"pattern" jsonschema:"description=Pattern of on/off durations in microseconds"`
	}
	gomcp.AddTool(s.mcp, &gomcp.Tool{
		Name:        "ir_transmit",
		Description: "Send an IR signal at the given carrier frequency with the specified on/off pattern. Currently stubbed because the pattern parameter requires constructing a Java int[] array handle.",
		Annotations: &gomcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			Title:           "IR Transmit",
		},
	}, func(ctx context.Context, req *gomcp.CallToolRequest, _ irTransmitInput) (*gomcp.CallToolResult, any, error) {
		return &gomcp.CallToolResult{
			Content: []gomcp.Content{&gomcp.TextContent{
				Text: "ir_transmit requires constructing a Java int[] array for the pattern parameter. " +
					"Use the jni_raw tool to create the int[] array handle, then call " +
					"ConsumerIrManager.transmit(frequency, pattern) via call_android_api.",
			}},
		}, nil, nil
	})

	// get_ir_info — read-only IR capabilities
	gomcp.AddTool(s.mcp, &gomcp.Tool{
		Name:        "get_ir_info",
		Description: "Check if the device has an IR emitter and get supported carrier frequencies. The carrier_frequencies_handle is a Java ConsumerIrManager.CarrierFrequencyRange[] handle.",
		Annotations: &gomcp.ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			Title:           "Get IR Info",
		},
	}, func(ctx context.Context, req *gomcp.CallToolRequest, _ irStatusInput) (*gomcp.CallToolResult, irStatusOutput, error) {
		client := irclient.NewClient(s.conn)

		var out irStatusOutput
		var err error

		out.HasIrEmitter, err = client.HasIrEmitter(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("has ir emitter: %w", err)
		}

		if out.HasIrEmitter {
			out.CarrierFreqHandle, err = client.GetCarrierFrequencies(ctx)
			if err != nil {
				return nil, out, fmt.Errorf("get carrier frequencies: %w", err)
			}
		}

		return nil, out, nil
	})
}

// ---------------------------------------------------------------------------
// Camera tools (CameraManager)
// ---------------------------------------------------------------------------

type listCamerasInput struct{}

type listCamerasOutput struct {
	CameraIDListHandle        int64 `json:"camera_id_list_handle"`
	ConcurrentCameraIDsHandle int64 `json:"concurrent_camera_ids_handle"`
}

func (s *Server) registerCameraTools() {
	// list_cameras — read-only: list available cameras
	gomcp.AddTool(s.mcp, &gomcp.Tool{
		Name: "list_cameras",
		Description: "List available cameras. Returns Java object handles for the camera ID list " +
			"(String[]) and concurrent camera ID set (Set<Set<String>>). " +
			"Use jni_raw to inspect the returned handles for individual camera IDs.",
		Annotations: &gomcp.ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			Title:           "List Cameras",
		},
	}, func(ctx context.Context, req *gomcp.CallToolRequest, _ listCamerasInput) (*gomcp.CallToolResult, listCamerasOutput, error) {
		client := cameraclient.NewClient(s.conn)

		var out listCamerasOutput
		var err error

		out.CameraIDListHandle, err = client.GetCameraIdList(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("get camera id list: %w", err)
		}

		out.ConcurrentCameraIDsHandle, err = client.GetConcurrentCameraIds(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("get concurrent camera ids: %w", err)
		}

		return nil, out, nil
	})

	// take_photo — stub: requires streaming RPC for camera capture pipeline
	type takePhotoInput struct {
		CameraID string `json:"camera_id" jsonschema:"description=Camera ID to capture from (from list_cameras)"`
	}
	gomcp.AddTool(s.mcp, &gomcp.Tool{
		Name: "take_photo",
		Description: "Take a photo with the specified camera. Currently stubbed because the full camera " +
			"capture pipeline requires the bidirectional Proxy streaming RPC to handle callbacks " +
			"(CameraDevice.StateCallback, CameraCaptureSession, ImageReader).",
		Annotations: &gomcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			Title:           "Take Photo",
		},
	}, func(ctx context.Context, req *gomcp.CallToolRequest, _ takePhotoInput) (*gomcp.CallToolResult, any, error) {
		return &gomcp.CallToolResult{
			Content: []gomcp.Content{&gomcp.TextContent{
				Text: "take_photo requires the bidirectional Proxy streaming RPC which is not yet " +
					"exposed via MCP. The full capture pipeline involves: " +
					"1) OpenCamera with a StateCallback, " +
					"2) Creating a CameraCaptureSession, " +
					"3) Setting up an ImageReader surface, " +
					"4) Issuing a capture request and waiting for the callback. " +
					"Use call_android_api or jni_raw for manual camera operations.",
			}},
		}, nil, nil
	})

	// capture_screen — stub: requires user interaction for MediaProjection consent
	type captureScreenInput struct{}
	gomcp.AddTool(s.mcp, &gomcp.Tool{
		Name: "capture_screen",
		Description: "Capture the device screen. Currently stubbed because screen capture requires " +
			"user consent via a system dialog (MediaProjection permission grant), which cannot " +
			"be automated without user interaction.",
		Annotations: &gomcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(true),
			Title:           "Capture Screen",
		},
	}, func(ctx context.Context, req *gomcp.CallToolRequest, _ captureScreenInput) (*gomcp.CallToolResult, any, error) {
		return &gomcp.CallToolResult{
			Content: []gomcp.Content{&gomcp.TextContent{
				Text: "capture_screen requires user consent via the MediaProjection permission dialog. " +
					"The workflow involves: " +
					"1) Call MediaProjectionManager.createScreenCaptureIntent() to get an Intent, " +
					"2) Start the intent via Activity.startActivityForResult() — the user must " +
					"tap 'Allow' on the system dialog, " +
					"3) Use the result to get a MediaProjection and create a VirtualDisplay. " +
					"This cannot be fully automated. Use call_android_api or jni_raw for " +
					"manual MediaProjection operations.",
			}},
		}, nil, nil
	})
}

// ---------------------------------------------------------------------------
// Scheduling tools (AlarmManager, JobScheduler)
// ---------------------------------------------------------------------------

type setAlarmInput struct {
	Type          int32 `json:"type" jsonschema:"description=Alarm type: 0=RTC_WAKEUP 1=RTC 2=ELAPSED_REALTIME_WAKEUP 3=ELAPSED_REALTIME"`
	TriggerMillis int64 `json:"trigger_millis" jsonschema:"description=Trigger time in milliseconds (RTC types: epoch millis; ELAPSED types: millis since boot)"`
}

type getNextAlarmInput struct{}

type getNextAlarmOutput struct {
	NextAlarmHandle      int64 `json:"next_alarm_handle"`
	CanScheduleExact     bool  `json:"can_schedule_exact_alarms"`
}

type manageJobsInput struct {
	Action string `json:"action" jsonschema:"enum=list,enum=cancel,enum=cancel_all,description=Action to perform: list pending jobs, cancel a specific job by ID, or cancel all jobs"`
	JobID  int32  `json:"job_id,omitempty" jsonschema:"description=Job ID for cancel action (ignored for list and cancel_all)"`
}

func (s *Server) registerSchedulingTools() {
	// set_alarm — stub: requires PendingIntent handle
	gomcp.AddTool(s.mcp, &gomcp.Tool{
		Name: "set_alarm",
		Description: "Set an alarm. The underlying AlarmManager.set() requires a PendingIntent (Java object handle) " +
			"as the third argument, which must be constructed via JNI. " +
			"Type values: 0=RTC_WAKEUP, 1=RTC, 2=ELAPSED_REALTIME_WAKEUP, 3=ELAPSED_REALTIME. " +
			"Currently stubbed — use jni_raw to create the PendingIntent, then call set_alarm " +
			"via call_android_api.",
		Annotations: &gomcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			Title:           "Set Alarm",
		},
	}, func(ctx context.Context, req *gomcp.CallToolRequest, in setAlarmInput) (*gomcp.CallToolResult, any, error) {
		return &gomcp.CallToolResult{
			Content: []gomcp.Content{&gomcp.TextContent{
				Text: fmt.Sprintf(
					"set_alarm requires a PendingIntent Java object handle as the third parameter to "+
						"AlarmManager.set(type=%d, triggerAtMillis=%d, pendingIntent). "+
						"Use jni_raw to: "+
						"1) Create an Intent targeting your BroadcastReceiver, "+
						"2) Create a PendingIntent via PendingIntent.getBroadcast(), "+
						"3) Then call AlarmManager.set() via call_android_api with the PendingIntent handle.",
					in.Type, in.TriggerMillis,
				),
			}},
		}, nil, nil
	})

	// get_next_alarm — read-only: query next alarm and scheduling permission
	gomcp.AddTool(s.mcp, &gomcp.Tool{
		Name: "get_next_alarm",
		Description: "Get the next scheduled alarm clock info and whether exact alarm scheduling is permitted. " +
			"The next_alarm_handle is a Java AlarmManager.AlarmClockInfo object handle (0 if no alarm set).",
		Annotations: &gomcp.ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			Title:           "Get Next Alarm",
		},
	}, func(ctx context.Context, req *gomcp.CallToolRequest, _ getNextAlarmInput) (*gomcp.CallToolResult, getNextAlarmOutput, error) {
		client := alarmclient.NewClient(s.conn)

		var out getNextAlarmOutput
		var err error

		out.NextAlarmHandle, err = client.GetNextAlarmClock(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("get next alarm clock: %w", err)
		}

		out.CanScheduleExact, err = client.CanScheduleExactAlarms(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("can schedule exact alarms: %w", err)
		}

		return nil, out, nil
	})

	// cancel_all_alarms — mutation: cancel all alarms
	type cancelAllAlarmsInput struct{}
	gomcp.AddTool(s.mcp, &gomcp.Tool{
		Name:        "cancel_all_alarms",
		Description: "Cancel all alarms set by this application via AlarmManager.cancelAll().",
		Annotations: &gomcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(true),
			Title:           "Cancel All Alarms",
		},
	}, func(ctx context.Context, req *gomcp.CallToolRequest, _ cancelAllAlarmsInput) (*gomcp.CallToolResult, any, error) {
		client := alarmclient.NewClient(s.conn)

		err := client.CancelAll(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("cancel all alarms: %w", err)
		}

		result := map[string]any{"cancelled": "all"}
		r, err := jsonResult(result)
		return r, nil, err
	})

	// manage_jobs — mutation: list/cancel/cancel_all jobs
	gomcp.AddTool(s.mcp, &gomcp.Tool{
		Name: "manage_jobs",
		Description: "Manage scheduled jobs via JobScheduler. Actions: " +
			"'list' returns the pending jobs handle (Java List<JobInfo>), " +
			"'cancel' cancels a specific job by ID, " +
			"'cancel_all' cancels all scheduled jobs.",
		Annotations: &gomcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			Title:           "Manage Jobs",
		},
	}, func(ctx context.Context, req *gomcp.CallToolRequest, in manageJobsInput) (*gomcp.CallToolResult, any, error) {
		client := jobclient.NewClient(s.conn)

		switch in.Action {
		case "list":
			handle, err := client.GetAllPendingJobs(ctx)
			if err != nil {
				return nil, nil, fmt.Errorf("get all pending jobs: %w", err)
			}
			result := map[string]any{
				"action":              "list",
				"pending_jobs_handle": handle,
			}
			r, err := jsonResult(result)
			return r, nil, err

		case "cancel":
			err := client.Cancel(ctx, in.JobID)
			if err != nil {
				return nil, nil, fmt.Errorf("cancel job %d: %w", in.JobID, err)
			}
			result := map[string]any{
				"action":       "cancel",
				"cancelled_id": in.JobID,
			}
			r, err := jsonResult(result)
			return r, nil, err

		case "cancel_all":
			err := client.CancelAll(ctx)
			if err != nil {
				return nil, nil, fmt.Errorf("cancel all jobs: %w", err)
			}
			result := map[string]any{
				"action":    "cancel_all",
				"cancelled": "all",
			}
			r, err := jsonResult(result)
			return r, nil, err

		default:
			return nil, nil, fmt.Errorf("unknown action %q: must be one of list, cancel, cancel_all", in.Action)
		}
	})
}

// ---------------------------------------------------------------------------
// Telecom tools (TelecomManager)
// ---------------------------------------------------------------------------

type callStateInput struct{}

type callStateOutput struct {
	IsInCall                   bool   `json:"is_in_call"`
	IsInManagedCall            bool   `json:"is_in_managed_call"`
	DefaultDialerPackage       string `json:"default_dialer_package"`
	SystemDialerPackage        string `json:"system_dialer_package"`
	CallCapableAccountsHandle  int64  `json:"call_capable_accounts_handle"`
	HasManageOngoingCallsPerm  bool   `json:"has_manage_ongoing_calls_permission"`
	IsTtySupported             bool   `json:"is_tty_supported"`
}

func (s *Server) registerTelecomTools() {
	// get_call_state — read-only: active call status and phone accounts
	gomcp.AddTool(s.mcp, &gomcp.Tool{
		Name: "get_call_state",
		Description: "Get active call status: whether a call is in progress, default/system dialer packages, " +
			"call-capable phone accounts handle (Java List<PhoneAccountHandle>), TTY support, " +
			"and manage-ongoing-calls permission.",
		Annotations: &gomcp.ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			Title:           "Get Call State",
		},
	}, func(ctx context.Context, req *gomcp.CallToolRequest, _ callStateInput) (*gomcp.CallToolResult, callStateOutput, error) {
		client := telecomclient.NewClient(s.conn)

		var out callStateOutput
		var err error

		out.IsInCall, err = client.IsInCall(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("is in call: %w", err)
		}

		out.IsInManagedCall, err = client.IsInManagedCall(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("is in managed call: %w", err)
		}

		out.DefaultDialerPackage, err = client.GetDefaultDialerPackage(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("get default dialer package: %w", err)
		}

		out.SystemDialerPackage, err = client.GetSystemDialerPackage(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("get system dialer package: %w", err)
		}

		out.CallCapableAccountsHandle, err = client.GetCallCapablePhoneAccounts(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("get call capable phone accounts: %w", err)
		}

		out.HasManageOngoingCallsPerm, err = client.HasManageOngoingCallsPermission(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("has manage ongoing calls permission: %w", err)
		}

		out.IsTtySupported, err = client.IsTtySupported(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("is tty supported: %w", err)
		}

		return nil, out, nil
	})

	// end_call — mutation, destructive: end the current call
	type endCallInput struct{}
	gomcp.AddTool(s.mcp, &gomcp.Tool{
		Name:        "end_call",
		Description: "End the current active call. Returns whether the call was successfully ended.",
		Annotations: &gomcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(true),
			Title:           "End Call",
		},
	}, func(ctx context.Context, req *gomcp.CallToolRequest, _ endCallInput) (*gomcp.CallToolResult, any, error) {
		client := telecomclient.NewClient(s.conn)

		ended, err := client.EndCall(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("end call: %w", err)
		}

		result := map[string]any{"call_ended": ended}
		r, err := jsonResult(result)
		return r, nil, err
	})

	// silence_ringer — mutation: silence the ringer
	type silenceRingerInput struct{}
	gomcp.AddTool(s.mcp, &gomcp.Tool{
		Name:        "silence_ringer",
		Description: "Silence the ringer if a call is ringing. Does not reject the call, only silences the ringtone.",
		Annotations: &gomcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			Title:           "Silence Ringer",
		},
	}, func(ctx context.Context, req *gomcp.CallToolRequest, _ silenceRingerInput) (*gomcp.CallToolResult, any, error) {
		client := telecomclient.NewClient(s.conn)

		err := client.SilenceRinger(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("silence ringer: %w", err)
		}

		result := map[string]any{"silenced": true}
		r, err := jsonResult(result)
		return r, nil, err
	})

	// accept_ringing_call — mutation: accept an incoming call
	type acceptCallInput struct{}
	gomcp.AddTool(s.mcp, &gomcp.Tool{
		Name:        "accept_ringing_call",
		Description: "Accept the currently ringing incoming call.",
		Annotations: &gomcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			Title:           "Accept Ringing Call",
		},
	}, func(ctx context.Context, req *gomcp.CallToolRequest, _ acceptCallInput) (*gomcp.CallToolResult, any, error) {
		client := telecomclient.NewClient(s.conn)

		err := client.AcceptRingingCall0(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("accept ringing call: %w", err)
		}

		result := map[string]any{"accepted": true}
		r, err := jsonResult(result)
		return r, nil, err
	})
}

// ---------------------------------------------------------------------------
// Input tools (InputMethodManager)
// ---------------------------------------------------------------------------

type getInputMethodsInput struct{}

type getInputMethodsOutput struct {
	IsAcceptingText          bool  `json:"is_accepting_text"`
	IsActive                 bool  `json:"is_active"`
	IsFullscreenMode         bool  `json:"is_fullscreen_mode"`
	CurrentInputMethodHandle int64 `json:"current_input_method_handle"`
	EnabledListHandle        int64 `json:"enabled_input_methods_handle"`
	AllListHandle            int64 `json:"all_input_methods_handle"`
}

type toggleKeyboardInput struct {
	ShowFlags int32 `json:"show_flags" jsonschema:"default=0,description=Show flags for ToggleSoftInput: 0=implicit 1=forced 2=not_always"`
	HideFlags int32 `json:"hide_flags" jsonschema:"default=0,description=Hide flags for ToggleSoftInput: 0=implicit 1=not_always"`
}

func (s *Server) registerInputTools() {
	// get_input_methods — read-only: list enabled input methods and status
	gomcp.AddTool(s.mcp, &gomcp.Tool{
		Name: "get_input_methods",
		Description: "Get input method status: whether a text field is accepting text, whether an IME is active, " +
			"fullscreen mode, and handles for the current, enabled, and all installed input methods " +
			"(Java InputMethodInfo objects). Use jni_raw to inspect the returned handles.",
		Annotations: &gomcp.ToolAnnotations{
			ReadOnlyHint:    true,
			DestructiveHint: boolPtr(false),
			Title:           "Get Input Methods",
		},
	}, func(ctx context.Context, req *gomcp.CallToolRequest, _ getInputMethodsInput) (*gomcp.CallToolResult, getInputMethodsOutput, error) {
		client := inputmethodclient.NewClient(s.conn)

		var out getInputMethodsOutput
		var err error

		out.IsAcceptingText, err = client.IsAcceptingText(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("is accepting text: %w", err)
		}

		out.IsActive, err = client.IsActive0(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("is active: %w", err)
		}

		out.IsFullscreenMode, err = client.IsFullscreenMode(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("is fullscreen mode: %w", err)
		}

		out.CurrentInputMethodHandle, err = client.GetCurrentInputMethodInfo(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("get current input method info: %w", err)
		}

		out.EnabledListHandle, err = client.GetEnabledInputMethodList(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("get enabled input method list: %w", err)
		}

		out.AllListHandle, err = client.GetInputMethodList(ctx)
		if err != nil {
			return nil, out, fmt.Errorf("get input method list: %w", err)
		}

		return nil, out, nil
	})

	// toggle_keyboard — mutation: show/hide/toggle the soft keyboard
	gomcp.AddTool(s.mcp, &gomcp.Tool{
		Name: "toggle_keyboard",
		Description: "Toggle the soft keyboard visibility using InputMethodManager.toggleSoftInput(). " +
			"show_flags: 0=SHOW_IMPLICIT, 1=SHOW_FORCED. " +
			"hide_flags: 0=HIDE_IMPLICIT_ONLY, 1=HIDE_NOT_ALWAYS.",
		Annotations: &gomcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			Title:           "Toggle Keyboard",
		},
	}, func(ctx context.Context, req *gomcp.CallToolRequest, in toggleKeyboardInput) (*gomcp.CallToolResult, any, error) {
		client := inputmethodclient.NewClient(s.conn)

		err := client.ToggleSoftInput(ctx, in.ShowFlags, in.HideFlags)
		if err != nil {
			return nil, nil, fmt.Errorf("toggle soft input: %w", err)
		}

		result := map[string]any{
			"toggled":    true,
			"show_flags": in.ShowFlags,
			"hide_flags": in.HideFlags,
		}
		r, err := jsonResult(result)
		return r, nil, err
	})

	// show_input_method_picker — mutation: show the system input method picker dialog
	type showIMEPickerInput struct{}
	gomcp.AddTool(s.mcp, &gomcp.Tool{
		Name:        "show_input_method_picker",
		Description: "Show the system input method picker dialog, allowing the user to switch between installed input methods.",
		Annotations: &gomcp.ToolAnnotations{
			ReadOnlyHint:    false,
			DestructiveHint: boolPtr(false),
			Title:           "Show Input Method Picker",
		},
	}, func(ctx context.Context, req *gomcp.CallToolRequest, _ showIMEPickerInput) (*gomcp.CallToolResult, any, error) {
		client := inputmethodclient.NewClient(s.conn)

		err := client.ShowInputMethodPicker(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("show input method picker: %w", err)
		}

		result := map[string]any{"picker_shown": true}
		r, err := jsonResult(result)
		return r, nil, err
	})
}
