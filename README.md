[![Build Status](https://travis-ci.org/simongui/tantrum.svg?branch=master)](https://travis-ci.org/simongui/tantrum)

# Tantrum
Tantrum is a Redis benchmarking orchestration tool. It can benchmark against multiple Redis servers and graph results and returns an imgur link to the graph.

# Usage
```
make
./tantrum --hosts=redis:localhost:6379,fastlane:localhost:6380
```

<img src="results.png"/>
