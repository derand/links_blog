from application import app
from flask import request, Response, session

from datetime import datetime, date
import pytz, time
import json
from application.mongodb import post_create
from application.config import date_to_db


def check_auth():
    return True
    if not session.has_key('id'):
        return json_response({ "code": 401, 'message': 'Unauthorized' })

def json_response(data):
    rtext = json.dumps(data)
    return Response(rtext, status=200, mimetype='application/json')


@app.route('/api/post_create', methods=['POST'])
def api_post_create():
    prms = request.form
    if not check_auth():
        return json_response({ "status": 401, 'message': 'Unauthorized' })
    url = prms.get('url')
    description = prms.get('description')
    day = prms.get('day')
    tags = prms.getlist('tag')
    print('url=%s description=%s day=%s tags=%s'%(url, description, day, tags))
    if not url or not description:
        return json_response({ "status": 400, 'message': 'All params does not setted' })
    if day:
        day = datetime.strptime(day, "%Y-%m-%d").date()
        day = date_to_db(day)

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
