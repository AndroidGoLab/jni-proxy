# Camera photo controls

`jnicli camera photo` captures the largest JPEG stream from the selected
Camera2 device. The default request is automatic exposure, flash off, and a
3-second AE warm-up. Replace `PHONE_IPV4:PORT` with the phone's configured
Wi-Fi IPv4 address and listener port; `--authority jniservice` keeps TLS
verification enabled over the direct LAN connection:

```bash
jnicli --profile pixel6a --addr PHONE_IPV4:PORT --authority jniservice --timeout 15s \
  camera photo --index 0 --output photo.jpg
```

The capture controls are:

| Option | Default | Meaning |
| --- | --- | --- |
| `--ae-timeout DURATION` | `3s` | Maximum AE convergence wait; `0` skips warm-up; maximum `30s`. |
| `--flash off\|auto\|on` | `off` | Flash policy for automatic exposure. |
| `--iso INTEGER` | `0` | Manual sensor ISO; `0` keeps AE enabled. |
| `--exposure DURATION` | `0s` | Manual sensor exposure time, such as `10ms`; `0` keeps AE enabled. |

The global `--timeout` must cover the selected AE warm-up and capture; the
examples use `15s` so the request deadline is longer than the default warm-up.

Manual exposure requires both `--iso` and `--exposure`, disables AE, and must
use `--flash off`. For example:

```bash
jnicli --profile pixel6a --addr PHONE_IPV4:PORT --authority jniservice --timeout 15s \
  camera photo --index 0 --flash off --iso 400 --exposure 10ms \
  --output manual.jpg
```

Invalid combinations are rejected by the CLI before invoking the remote camera
method, and the on-device helper validates them again before opening the camera.
