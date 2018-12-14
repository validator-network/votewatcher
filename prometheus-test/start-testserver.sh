#!/bin/bash

docker run -d -p 9090:9090 \
      -v  $(pwd)/prometheus-data:/prometheus-data \
       prom/prometheus --config.file=/prometheus-data/prometheus.yml

