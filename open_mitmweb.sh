#!/bin/bash -e

if [ "$1" = "-h" ] || [ "$1" = "--help" ];
then
    echo "Opens a browser with mitmweb. Then you can open a dump file made via COMPLEMENT_CRYPTO_MITMDUMP. (requires on PATH: docker)"
    exit 1
fi

# - use python3 instead of xdg-open because it's more portable (xdg-open doesn't work on MacOS). Sleep 1s and do it in the background.
(sleep 1 && python3 -m webbrowser http://localhost:1445) &

# - use same version as tests so we don't need to pull any new image. When the user CTRL+Cs this, the container quits.
docker run --rm -p 1445:8081 mitmproxy/mitmproxy:10.1.5  mitmweb --web-host 0.0.0.0