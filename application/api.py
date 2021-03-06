from application import app
from flask import request, Response, session

from datetime import datetime, date, timedelta
#import pytz, time
import json
import application.mongodb as db
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

def secret_hash(request):
    ip = get_remote_ip(request)
    user_agent = request.headers.get('User-Agent')
    secret = '%s%s%s'%(user_agent, ip, os.environ.get('GOOGLE_CLIENT_ID'))
    return hashlib.md5(secret.encode('utf-8')).hexdigest()

def is_loggedin(request):
    if '__sid' not in session:
        return False
    tm = int(time.time())
    md5_hash = secret_hash(request)
    if md5_hash == session.get('__sid'):
        if '__sidt' not in session or abs(session.get('__sidt')-tm) > 60*60*24:
            session.permanent = True
            app.permanent_session_lifetime = timedelta(days=3)
            session['__sidt'] = tm
            session['__sid'] = md5_hash
            if '__s' in session:
                session['__s'] = session['__s']
        return True
    if '__sid' in session:  del session['__sid']
    if '__sidt' in session: del session['__sidt']
    if '__s' in session: del session['__s']
    return False

def post_create(url, description=None, d=None, tags=tuple(), hidden=False):
    print('url=%s description=%s date=%s tags=%s hidden=%s'%(url, description, d, tags, hidden))
    if not url or not description:
        return { "status": 400, 'message': 'All params does not setted' }
    post = db.url_exists(url)
    if post:
        post.pop('_id', None)
        return { "status": 409, 'message': 'Duplicate url', 'post': post }
    if d:
        day = datetime.strptime(d, "%Y-%m-%d").date()
        day = common.date_to_db(day)
    else:
        day = None

    post = db.post_create(url=url, description=description, tags=tags, day=day, hidden=hidden)
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
    return rv

def is_safe_url(request, target):
    ref_url = urlparse(request.host_url)
    test_url = urlparse(urljoin(request.host_url, target))
    return test_url.scheme in ('http', 'https') and \
           ref_url.netloc == test_url.netloc

@app.route('/api/post_create', methods=['POST'])
def api_post_create():
    if not is_loggedin(request):
        return json_response({ "status": 401, 'message': 'Unauthorized' })
    prms = request.form
    rv = post_create(url=prms.get('url'), description=prms.get('description'), d=prms.get('date'), tags=prms.getlist('tag'), hidden=prms.get('hidden'))
    return json_response(rv)
