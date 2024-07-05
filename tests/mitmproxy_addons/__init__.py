from mitmproxy.addons import asgiapp
import subprocess
import sys

# some addons need non-std packages.
# Rather than try to bundle in `pip install` commands in the CMD section of the Dockerfile,
# just install them when the addon loads.
def install(package):
    subprocess.check_call([sys.executable, "-m", "pip", "install", package])

install("aiohttp")

from callback import Callback
from controller import MITM_DOMAIN_NAME, app

addons = [
    asgiapp.WSGIApp(app, MITM_DOMAIN_NAME, 80), # requests to this host will be routed to the flask app
    Callback(),
]
# testcontainers will look for this log line
print("loading complement crypto addons", flush=True)
