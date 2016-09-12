from flask import render_template, abort, request, redirect, url_for
from application import app
from application.mongodb import posts_last_days, posts_date
from application.config import db_day_to_human, db_day_to_linkdate, date_to_db

from urllib.parse import urlparse
import time
import datetime
import sys

@app.route('/', defaults={'year': None, 'month': None, 'day': None}, methods=['GET'])
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
        if p != 0:
            abort(400)
        d = date_to_db(d)
        posts = posts_date(d)
        if not posts:
            abort(404)
        val['one_date'] = '%04d-%02d-%02d'%(year, month, day)
    else:
        posts = posts_last_days(page=page, page_size=10)
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
                    'human_date': db_day_to_human(p.get('day')),
                    'link_date': db_day_to_linkdate(p.get('day')),
                    'posts': [p, ],
                })
    val['days'] = days
    if posts.get('pages'):
        pagination = []
        if page==0:
            pagination.append({ 'title': '«' })
        else:
            pagination.append({ 
                'title': '«',
                'url': '/?p=%d'%(page, )
            })
        for i in range(posts.get('pages', 0)):
            p = { 'title': i+1 }
            if i == page:
                p['active'] = True
            else:
                if i:
                    p['url'] = '/?p=%d'%(i+1, )
                else:
                    p['url'] = '/'
            pagination.append(p)
        if page==(posts.get('pages')-1):
            pagination.append({ 'title': '»' })
        else:
            pagination.append({ 
                'title': '»',
                'url': '/?p=%d'%(page+2, )
            })
        val['pagination'] = pagination
    return render_template('index.html', **val)
