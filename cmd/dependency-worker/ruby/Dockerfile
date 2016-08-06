FROM ruby:2.3-slim

RUN useradd --user-group --create-home --shell /bin/false checker

WORKDIR /home/checker
USER checker

ENTRYPOINT ["bundle", "outdated"]
