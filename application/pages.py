from flask import render_template
from application import app
from application.mongodb import last_days
from application.config import db_day_to_human, db_day_to_linkdate

from urllib.parse import urlparse
import time

@app.route('/')
def index():
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
