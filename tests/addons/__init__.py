from mitmproxy.addons import asgiapp

from status_code import StatusCode
from controller import MITM_DOMAIN_NAME, app

print("loading complement crypto addons")
addons = [
    asgiapp.WSGIApp(app, MITM_DOMAIN_NAME, 80), # requests to this host will be routed to the flask app
    StatusCode(),
]
