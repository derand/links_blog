#

FROM        python:3.9-slim-bullseye
MAINTAINER  Andrey Derevyagin


ENV MONGODB_CONNECTION \
    FLASK_SECRET \
    GOOGLE_CLIENT_ID \
    GOOGLE_CLIENT_SECRET \
    GOOGLE_USER_EMAIL

RUN apt-get update && \
    apt-get install -y build-essential libffi-dev libssl-dev ca-certificates && \
    rm -rf /var/lib/apt/lists/*

COPY . /app
WORKDIR /app

RUN pip install --no-cache-dir -U pip && \
    pip install --no-cache-dir -r requirements.txt

EXPOSE 80

CMD ["gunicorn", "-b", "0.0.0.0:80", "-c", "gunicorn.conf.py", "--log-level=debug", "application:app"]