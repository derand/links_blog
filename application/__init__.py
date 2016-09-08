from flask import Flask

app = Flask(__name__)
#app.config['DEBUG'] = True

#from application import urls
from application import pages
from application import api

from urllib.parse import quote

@app.template_filter()
def query_urlencode(value):
    if ' ' in value.strip():
        value = '"%s"'%value
    return quote(value)
