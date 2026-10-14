import os
import time
from gevent import monkey
monkey.patch_all()

# Wait for MongoDB connection to be ready
if os.environ.get('MONGODB_CONNECTION'):
    time.sleep(2)  # Short delay for database connectivity

# Worker configuration
timeout = 120
worker_class = 'gevent'
workers = 1
keepalive = 10