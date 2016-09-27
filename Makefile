project = tantrum
projectpath = ${PWD}
glidepath = ${PWD}/vendor/github.com/Masterminds/glide
redispath = ${PWD}/vendor/github.com/antirez/redis
wrkpath = ${PWD}/vendor/github.com/wg/wrk
wrk2path = ${PWD}/vendor/github.com/giltene/wrk2

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

$(wrkpath)/wrk:
	git clone https://github.com/wg/wrk.git $(wrkpath)
	cd $(wrkpath);make
	cp $(wrkpath)/wrk ./benchmark/wrk

$(wrk2path)/wrk:
	git clone https://github.com/giltene/wrk2.git $(wrk2path)
	cd $(wrk2path);make
	cp $(wrk2path)/wrk ./benchmark/wrk2

libs: $(glidepath)/glide $(redispath)/src/redis-benchmark $(wrkpath)/wrk $(wrk2path)/wrk
	$(glidepath)/glide install

deps: libs
