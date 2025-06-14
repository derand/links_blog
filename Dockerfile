#

FROM        python:3.9-slim-buster
MAINTAINER  Andrey Derevyagin

COPY . /app
WORKDIR /app

ENV MONGODB_CONNECTION \
    FLASK_SECRET \
    GOOGLE_CLIENT_ID \
    GOOGLE_CLIENT_SECRET \
    GOOGLE_USER_EMAIL

RUN pip install --no-cache-dir -r requirements.txt

EXPOSE 80

CMD ["gunicorn", "-b", "0.0.0.0:80", "--log-level=debug", "application:app"]
