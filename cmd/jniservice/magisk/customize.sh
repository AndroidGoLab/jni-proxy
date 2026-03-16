#!/system/bin/sh
# Magisk module install script.
# MODPATH is set by Magisk to the module's install directory.

ui_print "Installing jniservice..."
ui_print "  gRPC server will start on next boot"
ui_print "  Config: <module_dir>/jniservice.env"
ui_print "  Logs:   <module_dir>/jniservice.log"
ui_print "  Data:   <module_dir>/data/"

# Set permissions on service.sh.
set_perm "$MODPATH/service.sh" 0 0 0755
set_perm_recursive "$MODPATH/jniservice" 0 0 0755 0644
