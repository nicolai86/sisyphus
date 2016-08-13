FROM node:6.3-slim

RUN useradd --user-group --create-home --shell /bin/false checker

WORKDIR /home/checker
USER checker

RUN npm set progress=false

ENTRYPOINT npm outdated --json 2>/dev/null > outdated.json
