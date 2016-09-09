from flask import render_template, abort
from application import app
from application.mongodb import posts_last_days, posts_date
from application.config import db_day_to_human, db_day_to_linkdate, date_to_db

from urllib.parse import urlparse
import time
import datetime

@app.route('/', defaults={'year': None, 'month': None, 'day': None})
@app.route('/<int:year>-<int:month>-<int:day>.html')
def index(year, month, day):
    val = {}
    if year and month and day:
        try:
            d = datetime.datetime(year, month, day).date()
        except ValueError:
            abort(404)
        d = date_to_db(d)
        posts = posts_date(d)
        if not posts:
            abort(404)
        val['one_date'] = '%04d-%02d-%02d'%(year, month, day)
    else:
        posts = posts_last_days()
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
    return render_template('index.html', **val)

