import datetime
import shlex

timezone='Europe/Kiev'

def db_day_to_human(db_date):
    #dt = datetime.datetime(year, 1, 1) + datetime.timedelta(days - 1)
    dt = datetime.datetime.strptime('%07d'%db_date, '%Y%j')
    return dt.strftime('%d %b %Y')

def db_day_to_linkdate(db_date):
    #dt = datetime.datetime(year, 1, 1) + datetime.timedelta(days - 1)
    dt = datetime.datetime.strptime('%07d'%db_date, '%Y%j')
    return dt.strftime('%Y-%m-%d')

def date_to_db(date):
    return date.year*1000 + date.timetuple().tm_yday


def pagination_dict(page, pages, center_side_count=2, url_prefix='/'):
    '''
        center_side_count — count pages to show left|right from current page
    '''
    # « 1 2 3 4 5 -6- 7 8 9 10 11 12 »
    def append_page(pagination, title, url=None, active=False):
        p = { 'title': title }
        if url and not active:
            p['url'] = url
        if active:
            #p['active'] = active
            p['class'] = 'active'
        pagination.append(p)
    def url_for_page(page, url_prefix):
        delimiter = '&' if '?' in url_prefix else '?'
        return url_prefix if page==0 else '%s%sp=%d'%(url_prefix, delimiter, page+1)

    pagination = []
    if page==0:
        pagination.append({ 'title': '«', 'class': 'disabled' })
    else:
        pagination.append({ 
            'title': '«',
            'url': url_for_page(page=page-1, url_prefix=url_prefix)
        })
    center_side_count_left = center_side_count_right = center_side_count
    if page < center_side_count:
        center_side_count_right += (center_side_count_right-page)
    if page > (pages-2-center_side_count):
        center_side_count_left += (page - pages + 3)
    if page < center_side_count_left+3:
        for i in range(page):
            append_page(pagination=pagination, title=i+1, url=url_for_page(page=i, url_prefix=url_prefix), active=(i==page))
    else:
        append_page(pagination=pagination, title='1', url=url_for_page(page=0, url_prefix=url_prefix))
        append_page(pagination=pagination, title='...')
        for i in range(page-center_side_count_left, page):
            append_page(pagination=pagination, title=i+1, url=url_for_page(page=i, url_prefix=url_prefix), active=(i==page))
    append_page(pagination=pagination, title=page+1, active=True)
    if page > (pages - center_side_count_right - 4):
        for i in range(page+1, pages):
            append_page(pagination=pagination, title=i+1, url=url_for_page(page=i, url_prefix=url_prefix), active=(i==page))
    else:
        for i in range(page+1, page+1+center_side_count_right):
            append_page(pagination=pagination, title=i+1, url=url_for_page(page=i, url_prefix=url_prefix), active=(i==page))
        append_page(pagination=pagination, title='...')
        append_page(pagination=pagination, title=pages, url=url_for_page(page=pages-1, url_prefix=url_prefix))
    if page>=(pages-1):
        pagination.append({ 'title': '»', 'class': 'disabled' })
    else:
        pagination.append({ 
            'title': '»',
            'url': url_for_page(page=page+1, url_prefix=url_prefix)
        })
    return pagination

def split_query(q):
    '''
        q — user query

        Return dictionary:
            {
                'status': int
                'q': [],
                'tags': [],
                'date': []
            }
    '''
    try:
        qs = shlex.split(q)
    except ValueError as e:
        try:
            qs = shlex.split(q.replace('\'', ''))
        except ValueError as e:
            return { 'status': 400 }
    rv = {
        'status': 200,
        'tags': [],
        'date': [],
        'q': []
    }
    for w in qs:
        if w.lower().startswith('tag:'):
            rv['tags'].append(w[len('tag:'):])
        elif w.lower().startswith('date:'):
            rv['date'].append(w[len('date:'):])
        else:
            rv['q'].append(w)
    return rv

if __name__ == "__main__":
    import json

    #p = pagination_dict(page=5, pages=12, center_side_count=2)
    #print(json.dumps(p, sort_keys=True, indent=4))

    qo = split_query('tag:"neural network" яндекс')
    print(json.dumps(qo, sort_keys=True, indent=4))
