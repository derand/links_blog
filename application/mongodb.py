from application import app
from flask_pymongo import PyMongo
from pymongo import DESCENDING, ASCENDING
import os
import time, pytz
from datetime import datetime, date
import application.common as common

mongo = None
if os.environ.get('MONGODB_CONNECTION') is not None:
    app.config['MONGO_URI'] = os.environ.get('MONGODB_CONNECTION')
    mongo = PyMongo(app)


@app.before_first_request
def _run_on_start():
    if mongo:
        print(mongo.db)
        if mongo.db:
            mongo.db.links.create_index([('description', 'text'), ('url', 'text'), ('tags', 'text')])

'''
{
    "_id": {
        "$oid": "57b9f673dcba0f5b14f0fb35"
    },
    "created": 0,
    "day": 0,
    "url": "http://iichan-rpg-stat.herokuapp.com/threads",
    "description": "мой архив тредов с имиджборд",
    "tags": [
        "rpg",
        "text",
        "images"
    ],
    "hidden": false
}
'''

def posts_last_days(page=0, page_size=10, get_hidden=False):
    rv = None
    if mongo and mongo.db:
        if get_hidden:
            arr = mongo.db.links.distinct('day')
        else:
            arr = mongo.db.links.find({ '$or': [ { "hidden": { "$exists": False } }, { "hidden": False } ] }).distinct('day')
        days = sorted(arr, reverse=True)
        pages = len(arr) // page_size
        if len(arr) % page_size:
            pages += 1
        if page > 0 and page*page_size < len(days):
            days = days[page*page_size:]
        elif page:
            return { 'pages': pages }
        if len(days) > page_size:
            days = days[:page_size]
        query = {
            'day': { "$in": days },
        }
        if not get_hidden:
            query['$or'] = [ { "hidden": { "$exists": False } }, { "hidden": False } ]
        cursor = mongo.db.links.find(query, sort=[('day', DESCENDING), ('created', ASCENDING)])
        rv = {
            'posts': tuple(cursor),
            'pages': pages,
            'days': days,
        }
    return rv

def posts_date(db_day, get_hidden=False):
    rv = None
    if mongo and mongo.db:
        query = {
            'day': db_day,
        }
        if not get_hidden:
            query['$or'] = [ { "hidden": { "$exists": False } }, { "hidden": False } ]
        cursor = mongo.db.links.find(query, sort=[('day', DESCENDING), ('created', ASCENDING)])
        rv = {
            'posts': tuple(cursor),
            'days': db_day,
        }
    return rv

def posts_search(q_object, page=0, page_size=50, get_hidden=False):
    '''
        q_object - query object (dict) from string splitted by common.split_query
    '''
    rv = None
    if mongo and mongo.db:
        and_query = []
        if len(q_object.get('tags')):
            for t in q_object.get('tags'):
                and_query.append({ 
                    "tags": {
                        "$elemMatch": {
                            "$eq": t
                        }
                    }})
        if len(q_object.get('q')):
            s = ''
            for q in q_object.get('q'):
                if ' ' in q or '\t' in q:
                    s = '%s "%s"'%(s, q)
                else:
                    s = '%s %s'%(s, q)
            and_query.append({ 
                "$text": {
                    "$search": s
                }})
        query = {} if len(and_query)==0 else and_query[0] if len(and_query)==1 else { "$and": and_query }
        if not get_hidden:
            if '$or' in query:
                query['$or'].append([ { "hidden": { "$exists": False } }, { "hidden": False } ])
            else:
                query['$or'] = [ { "hidden": { "$exists": False } }, { "hidden": False } ]
        #print(query)
        cursor = mongo.db.links.find(query)
        count = cursor.count()
        pages = count // page_size
        if count % page_size:
            pages += 1
        rv = {
            'pages': pages,
            'count': count,
        }
        if page > 0 and page*page_size > count:
            return rv
        cursor = mongo.db.links.find(query, skip=page*page_size, limit=page_size, sort=[('day', DESCENDING), ])
        rv['posts'] = tuple(cursor)
    return rv    

def post_create(url=None, description=None, tags=[], day=None, hidden=False):
    post = None
    if mongo and mongo.db:
        tm = int(time.time())
        post = {
            'created': tm,
            'url': url,
            'description': description,
        }
        if day is None:
            utc_dt = datetime.utcfromtimestamp(tm).replace(tzinfo=pytz.utc)
            tz = pytz.timezone(common.timezone)
            dt = utc_dt.astimezone(tz).date()
            post['day'] = common.date_to_db(dt)
        else:
            post['day'] = day
        if tags:
            post['tags'] = tags
        if hidden:
            post['hidden'] = True
        mongo.db.links.insert(post)
    return post

def url_exists(url):
    if mongo and mongo.db:
        return mongo.db.links.find_one({ 'url': url })
    return False

def has_user_access(credentials_str, user_info):
    return user_info.get('verified_email') and user_info.get('email') == os.environ.get('GOOGLE_USER_EMAIL')
    '''
    if mongo.db:
        pass
    return False
    '''
