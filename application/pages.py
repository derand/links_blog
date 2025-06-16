from flask import render_template, abort, request, redirect, url_for, session, make_response
from application import app
import application.mongodb as db
import application.common as common
import application.api as api

from urllib.parse import urlparse
import time
import datetime
import sys, os
import copy
from urllib.parse import quote
import requests
from google.oauth2 import id_token
from google.auth.transport import requests as google_requests
from google.auth.exceptions import GoogleAuthError


@app.route('/', defaults={'year': None, 'month': None, 'day': None}, methods=['GET'])
@app.route('/index.html', defaults={'year': None, 'month': None, 'day': None}, methods=['GET'])
@app.route('/<int:year>-<int:month>-<int:day>.html', methods=['GET'])
def index(year, month, day):
    #print(request.url, request.base_url)
    isloggedin = True if api.is_loggedin(request) else False
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
        posts = db.posts_date(d, get_hidden=isloggedin)
        if not posts:
            abort(404)
        val['one_date'] = '%04d-%02d-%02d'%(year, month, day)
    else:
        posts = db.posts_last_days(page=page, page_size=10, get_hidden=isloggedin)
        if posts and page and page >= posts.get('pages', sys.maxsize):
            return redirect(url_for('.index'), code=302)
    days = []
    if posts:
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
        if posts.get('pages'):
            val['pagination'] = common.pagination_dict(page=page, pages=posts.get('pages'), center_side_count=2, url_prefix='/index.html')
    val['days'] = days
    val['is_loggedin'] = isloggedin
    val['redirect_url'] = request.base_url
    return render_template('index.html', **val)

@app.route('/search', methods=['GET'])
def search():
    isloggedin = True if api.is_loggedin(request) else False
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
        posts = db.posts_search(q_object, page=page, page_size=50, get_hidden=isloggedin)

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
    val['is_loggedin'] = isloggedin
    return render_template('search.html', **val)

@app.route('/post', methods=['POST'])
def post():
    if not api.is_loggedin(request):
        return abort(401)
    prms = request.form
    rv = api.post_create(url=prms.get('url'), description=prms.get('description'), d=prms.get('date'), tags=prms.getlist('tag'), hidden=prms.get('hidden'))
    if rv.get('status', 200) != 200:
        return abort(rv.get('status'))
    if prms.get('redirect_url') and api.is_safe_url(request, prms.get('redirect_url')):
        return redirect(prms.get('redirect_url'))
    return redirect(url_for('index'))

@app.route('/health')
def health_check():
    try:
        # Simple DB check
        if db.mongo and db.mongo.cx:
            db.mongo.cx.server_info()
        return "OK", 200
    except Exception:
        return "Database error", 500

@app.route('/login')
def login():
    if api.is_loggedin(request):
        return 'You is Logged In.'
    return redirect(url_for('oauth2callback'))
    '''
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
    '''

@app.route('/logout')
def logout():
    if '__sid' in session:  del session['__sid']
    if '__sidt' in session: del session['__sidt']
    if '__s' in session: del session['__s']
    return redirect(url_for('index'))

@app.route('/oauth2callback')
def oauth2callback():
    if 'code' not in request.args:
        # Generate redirect URI with forced HTTPS
        redirect_uri = url_for('oauth2callback', _external=True)
        if app.config.get('PREFERRED_URL_SCHEME', 'http') == 'https':
            redirect_uri = redirect_uri.replace('http://', 'https://', 1)
        app.logger.critical(f"Generated Redirect URI: {redirect_uri}")
        
        # Step 1: Redirect to Google's authorization page
        google_auth_url = (
            "https://accounts.google.com/o/oauth2/v2/auth?"
            "response_type=code&"
            f"client_id={os.environ.get('GOOGLE_CLIENT_ID')}&"
            f"redirect_uri={redirect_uri}&"
            "scope=openid%20email&"
            "access_type=offline&"
            "prompt=consent"
        )
        app.logger.debug(f"Google Auth URL: {google_auth_url}")
        return redirect(google_auth_url)
    else:
        try:
            # Generate redirect URI consistently
            redirect_uri = url_for('oauth2callback', _external=True)
            if app.config.get('PREFERRED_URL_SCHEME', 'http') == 'https':
                redirect_uri = redirect_uri.replace('http://', 'https://', 1)
            
            auth_code = request.args.get('code')
            token_url = "https://oauth2.googleapis.com/token"
            
            # Use application/x-www-form-urlencoded format
            token_data = {
                'code': auth_code,
                'client_id': os.environ.get('GOOGLE_CLIENT_ID'),
                'client_secret': os.environ.get('GOOGLE_CLIENT_SECRET'),
                'redirect_uri': redirect_uri,
                'grant_type': 'authorization_code'
            }
            
            # Add debug logging
            app.logger.debug(f"Token exchange data: {token_data}")
            
            # Make request with proper headers
            token_response = requests.post(
                token_url,
                data=token_data,
                headers={'Content-Type': 'application/x-www-form-urlencoded'},
                timeout=10
            )

            # Check for errors in response
            if token_response.status_code != 200:
                app.logger.error(
                    f"Token exchange failed: {token_response.status_code} "
                    f"{token_response.text}"
                )
                return "Authentication failed: Token exchange error", 400
            
            token_json = token_response.json()
            id_token_str = token_json['id_token']

            try:
                id_info = id_token.verify_oauth2_token(
                    token_json['id_token'],
                    google_requests.Request(),
                    os.environ.get('GOOGLE_CLIENT_ID')
                )
                app.logger.debug(f"ID token info: {id_info}")
            except ValueError as e:
                app.logger.error(f"ID token verification failed: {str(e)}")
                return "Authentication failed: Invalid ID token", 401

            
            # Verify ID token
            id_info = id_token.verify_oauth2_token(
                id_token_str,
                google_requests.Request(),
                os.environ.get('GOOGLE_CLIENT_ID')
            )
            
            # Extract user info
            user_info = {
                'email': id_info['email'],
                'name': id_info.get('name', ''),
                'picture': id_info.get('picture', ''),
                'verified_email': id_info.get('email_verified', False),
            }
            
            #app.logger.critical(f"Token: {token_json}")
            #app.logger.critical(f"ID: {id_info}")
            #app.logger.critical(f"User: {user_info}")

            # Check user access
            access = db.has_user_access(token_json, user_info)

            if access:
                md5_hash = api.secret_hash(request)
                session.permanent = True
                app.permanent_session_lifetime = datetime.timedelta(days=3)
                session['__sid'] = md5_hash
                session['__sidt'] = int(time.time())
                if access is not True:
                    session['__s'] = access
            else:
                return 'You don\'t have permissions on this site.' 

            return redirect(url_for('index'))
            
        except requests.exceptions.RequestException as e:
            app.logger.error(f"OAuth request failed: {str(e)}")
            return "Authentication failed: Connection error", 500
        except GoogleAuthError as e:
            app.logger.error(f"Google authentication error: {str(e)}")
            return "Authentication failed: Invalid credentials", 401
        except Exception as e:
            app.logger.error(f"Unexpected error: {str(e)}")
            return "Authentication failed", 500
