from flask import Flask
import os

from werkzeug.middleware.proxy_fix import ProxyFix

app = Flask(__name__)
#app.config['DEBUG'] = True
app.secret_key = os.environ.get('FLASK_SECRET')

# Add after app creation
app.wsgi_app = ProxyFix(
    app.wsgi_app, 
    x_for=1, 
    x_proto=1, 
    x_host=1, 
    x_prefix=1
)

# Force HTTPS in URL generation
app.config['PREFERRED_URL_SCHEME'] = 'https'

#from application import urls
from application import pages
from application import api

from urllib.parse import quote

@app.template_filter()
def query_urlencode(value):
    if ' ' in value.strip():
        value = '"%s"'%value
    return quote(value)

