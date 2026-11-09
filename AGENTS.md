# Agent instructions

## Home Assistant

- The local Home Assistant setup (`server/homeassistant`, `make ha-up`, `make services-up`) is **for tests only**.
- Real testing during development is done against the home Home Assistant, unless stated otherwise.
- The local Home Assistant container is expected to be stopped; do not assume it should be started, and do not start it without being asked.
- Do not treat closed ports 8123/8080 as an environment failure.
