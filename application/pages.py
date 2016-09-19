from flask import render_template, abort, request, redirect, url_for, session
from application import app
#from application.mongodb import posts_last_days, posts_date
import application.mongodb as db
import application.common as common

from urllib.parse import urlparse
import time
import datetime
import sys
import copy
from urllib.parse import quote
from oauth2client import client
from apiclient import discovery
import os
import httplib2

@app.route('/', defaults={'year': None, 'month': None, 'day': None}, methods=['GET'])
@app.route('/index.html', defaults={'year': None, 'month': None, 'day': None}, methods=['GET'])
@app.route('/<int:year>-<int:month>-<int:day>.html', methods=['GET'])
def index(year, month, day):
    #print(request.url, request.base_url)
    val = {}
    try:
        page = int(request.args.get('p', 1)) - 1 
    except ValueError:
        page = 0
    if year and month and day:
        try:
            d = datetime.datetime(year, month, day).date()
        except ValueError:
            abort(404)
        if page != 0:
            abort(400)
        d = common.date_to_db(d)
        posts = db.posts_date(d)
        if not posts:
            abort(404)
        val['one_date'] = '%04d-%02d-%02d'%(year, month, day)
    else:
        posts = db.posts_last_days(page=page, page_size=10)
        if posts and page and page >= posts.get('pages', sys.maxsize):
            return redirect(url_for('.index'), code=302)
    days = []
    for p in posts.get('posts', tuple()):
        if 'short_url' not in p:
            parsed_uri = urlparse(p.get('url'))
            p['short_url'] = '{uri.netloc}'.format(uri=parsed_uri)
        tmp = tuple(filter(lambda el: el.get('day')==p.get('day'), days))
        if len(tmp):
            tmp[0]['posts'].append(p)
        else:
            days.append({
                    'day': p.get('day'),
                    'human_date': common.db_day_to_human(p.get('day')),
                    'link_date': common.db_day_to_linkdate(p.get('day')),
                    'posts': [p, ],
                })
    val['days'] = days
    if posts.get('pages'):
        val['pagination'] = common.pagination_dict(page=page, pages=posts.get('pages'), center_side_count=2, url_prefix='/index.html')
    return render_template('index.html', **val)

@app.route('/search', methods=['GET'])
def search():
    q = request.args.get('q')
    try:
        page = int(request.args.get('p', 1)) - 1 
    except ValueError:
        page = 0
    val = {}
    posts = {}
    if q is not None:
        q_object = common.split_query(q)
        if q_object.get('status', 200) != 200:
            abort(q_object.get('status'))
        posts = db.posts_search(q_object, page=page, page_size=50)

    val['posts'] = list()
    for p in posts.get('posts', tuple()):
        tmp = copy.copy(p)
        tmp.pop('_id', None)
        if 'short_url' not in tmp:
            parsed_uri = urlparse(tmp.get('url'))
            tmp['short_url'] = '{uri.netloc}'.format(uri=parsed_uri)
        tmp['link_date'] = common.db_day_to_linkdate(tmp.get('day'))
        val['posts'].append(tmp)
    if q:
        val['q'] = q
    if posts.get('count') is not None:
        val['search_count'] = posts.get('count')
    #print(val)
    if posts.get('pages'):
        url_prefix = '/search'
        if q:
            url_prefix += '?q=%s'%quote(q)
        val['pagination'] = common.pagination_dict(page=page, pages=posts.get('pages'), center_side_count=2, url_prefix=url_prefix)
    return render_template('search.html', **val)

@app.route('/login')
def login():
    if 'credentials' not in session:
        return redirect(url_for('oauth2callback'))
    credentials = client.OAuth2Credentials.from_json(session['credentials'])
    if credentials is None or credentials.invalid:
        print('credentials.invalid')
        return redirect(url_for('oauth2callback'))
    if credentials.access_token_expired:
        print('access_token_expired')
        try:
            credentials.refresh(httplib2.Http())
        except oauth2client.client.HttpAccessTokenRefreshError:
            print('error')
            return redirect(url_for('oauth2callback'))
        session['credentials'] = credentials.to_json()
        print('credentials:', credentials.to_json())
    http_auth = credentials.authorize(httplib2.Http())
    user_info_service = discovery.build(
        serviceName='oauth2', version='v2',
        http=http_auth)
    user_info = None
    try:
        user_info = user_info_service.userinfo().get().execute()
    except errors.HttpError as e:
        logging.error('An error occurred: %s', e)
    print(user_info)
    return 'OK'

@app.route('/oauth2callback')
def oauth2callback():
    constructor_kwargs = {
                'redirect_uri': url_for('oauth2callback', _external=True),
                'auth_uri': 'https://accounts.google.com/o/oauth2/auth',
                'token_uri': 'https://accounts.google.com/o/oauth2/token',
                'login_hint': None,
                'prompt': 'consent',
            }
    flow = client.OAuth2WebServerFlow(os.environ.get('GOOGLE_CLIENT_ID'), os.environ.get('GOOGLE_CLIENT_SECRET'), 'email', **constructor_kwargs)
    if 'code' not in request.args:
        auth_uri = flow.step1_get_authorize_url()
        return redirect(auth_uri)
    else:
        auth_code = request.args.get('code')
        credentials = flow.step2_exchange(auth_code)
        session['credentials'] = credentials.to_json()
        print('credentials:', credentials.to_json())
        return redirect(url_for('index'))

