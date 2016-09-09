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
