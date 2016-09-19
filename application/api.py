from application import app
from flask import request, Response, session

from datetime import datetime, date
#import pytz, time
import json
from application.mongodb import post_create
import application.common as common
import os
import hashlib
import time


def json_response(data):
    rtext = json.dumps(data)
    return Response(rtext, status=200, mimetype='application/json')

def get_remote_ip(request):
    if not request.headers.getlist("X-Forwarded-For"):
        remote_ip = request.remote_addr
    else:
        remote_ip = request.headers.getlist("X-Forwarded-For")[0]
    return remote_ip

def is_logged(request):
    if '__sid' not in session:
        return False
    tm = int(time.time())
    if '__exp' in session: 
        pass
    ip = get_remote_ip(request)
    user_agent = request.headers.get('User-Agent')
    secret = '%s%s%s'%(user_agent, ip, os.environ.get('GOOGLE_CLIENT_ID'))
    m = hashlib.md5(secret.encode('utf-8'))
    if m.hexdigest() == session.get('__sid'):
        session['__exp'] = tm + 3*60*60*24
        return True
    return False


@app.route('/api/post_create', methods=['POST'])
def api_post_create():
    prms = request.form
    if not is_logged(request):
        return json_response({ "status": 401, 'message': 'Unauthorized' })
    url = prms.get('url')
    description = prms.get('description')
    d = prms.get('date')
    tags = prms.getlist('tag')
    print('url=%s description=%s date=%s tags=%s'%(url, description, d, tags))
    if not url or not description:
        return json_response({ "status": 400, 'message': 'All params does not setted' })
    if d:
        day = datetime.strptime(d, "%Y-%m-%d").date()
        day = common.date_to_db(day)
    else:
        day = None

    post = post_create(url=url, description=description, tags=tags, day=day)
    if post:
        post.pop('_id', None)
        rv = {
            'status': 200,
            'post': post,
        }
    else:
        rv = {
            'status': 500,
        }
    return json_response(rv)
