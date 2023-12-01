from mitmproxy.addons import asgiapp

from status_code import StatusCode
from controller import MITM_DOMAIN_NAME, app

addons = [
    asgiapp.WSGIApp(app, MITM_DOMAIN_NAME, 80), # requests to this host will be routed to the flask app
    StatusCode(),
]
# testcontainers will look for this log line
print("loading complement crypto addons", flush=True)
