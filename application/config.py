import datetime

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


def pagination_dict(page, pages):
    # « 1 2 3 4 5 -6- 7 8 9 10 11 12 »
    def append_page(pagination, title, url=None, active=False):
        p = { 'title': title }
        if url and not active:
            p['url'] = url
        if active:
            p['active'] = active
        pagination.append(p)

    pagination = []
    if page==0:
        pagination.append({ 'title': '«' })
    else:
        pagination.append({ 
            'title': '«',
            'url': '/?p=%d'%(page, )
        })
    if page < 4:
        for i in range(page):
            append_page(pagination=pagination, title=i+1, url='/?p=%d'%(i+1, ), active=(i==page))
    else:
        append_page(pagination=pagination, title='1', url='/?p=1')
        append_page(pagination=pagination, title='...')
        for i in range(page-2, page):
            append_page(pagination=pagination, title=i+1, url='/?p=%d'%(i+1, ), active=(i==page))
    append_page(pagination=pagination, title=page+1, active=True)
    if page > (pages - 6):
        for i in range(page+1, pages):
            append_page(pagination=pagination, title=i+1, url='/?p=%d'%(i+1, ), active=(i==page))
    else:
        for i in range(page+1, page+3):
            append_page(pagination=pagination, title=i+1, url='/?p=%d'%(i+1, ), active=(i==page))
        append_page(pagination=pagination, title='...')
        append_page(pagination=pagination, title=pages, url='/?p=%d'%pages)
    '''
    for i in range(pages):
        p = { 'title': i+1 }
        if i == page:
            p['active'] = True
        else:
            if i:
                p['url'] = '/?p=%d'%(i+1, )
            else:
                p['url'] = '/'
        pagination.append(p)
    '''
    if page==(pages-1):
        pagination.append({ 'title': '»' })
    else:
        pagination.append({ 
            'title': '»',
            'url': '/?p=%d'%(page+2, )
        })
    return pagination


if __name__ == "__main__":
    import json
    p = pagination_dict(8, 12)
    print(json.dumps(p, sort_keys=True, indent=4))
