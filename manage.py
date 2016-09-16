#!/usr/bin/env python
# -*- coding: utf-8 -*-

from application import app

#import os
#print(os.environ.get('MONGODB_CONNECTION'))

if __name__ == "__main__":
    app.config['DEBUG'] = True
    #app.run(host='127.0.0.1', port=8000)
    app.run(host='0.0.0.0', port=8000)
