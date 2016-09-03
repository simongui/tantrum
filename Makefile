project = tantrum
projectpath = ${PWD}
glidepath = ${PWD}/vendor/github.com/Masterminds/glide
redispath = ${PWD}/vendor/github.com/antirez/redis

target:
	@go build

test:
	@go test

integration: test
	@go test -tags=integration

$(glidepath)/glide:
	git clone https://github.com/Masterminds/glide.git $(glidepath)
	cd $(glidepath);make build
	cp $(glidepath)/glide .

$(redispath)/src/redis-benchmark:
	git clone https://github.com/antirez/redis.git $(redispath)
	cd $(redispath);make
	cp $(redispath)/src/redis-benchmark .

libs: $(glidepath)/glide $(redispath)/src/redis-benchmark
	$(glidepath)/glide install

deps: libs
