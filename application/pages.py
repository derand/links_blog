from flask import render_template, abort
from application import app
from application.mongodb import last_days, date
from application.config import db_day_to_human, db_day_to_linkdate, date_to_db

from urllib.parse import urlparse
import time
import datetime

@app.route('/', defaults={'year': None, 'month': None, 'day': None})
@app.route('/<int:year>-<int:month>-<int:day>.html')
def index(year, month, day):
    if year and month and day:
        try:
            day = datetime.datetime(year, month, day).date()
        except ValueError:
            abort(404)
        day = date_to_db(day)
        posts = date(day)
        if not posts:
            abort(404)
    else:
        posts = last_days()
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
    val = {
        #'title': 'Threads',
        'days': days,
    }
    #print(posts)
    #print(val)
    return render_template('index.html', **val)

