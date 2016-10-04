#

FROM        python:3
MAINTAINER  Andrey Derevyagin

RUN mkdir -p /usr/src
WORKDIR /usr/src
COPY . /usr/src

RUN pip install --no-cache-dir -r requirements.txt

EXPOSE 8000

CMD ["gunicorn", "-b", "0.0.0.0:8000", "--log-level=debug", "application:app"]
