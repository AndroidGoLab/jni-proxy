package server

// PermissionRequestNotifier is called when a new permission request is
// created. The implementation should notify the device user (e.g. launch
// a dialog Activity or push a notification).
type PermissionRequestNotifier func(requestID int64, clientID string, methods []string)
