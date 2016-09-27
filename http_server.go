package main

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/valyala/fasthttp"
)

var pools map[int64]*redis.Pool

func newPool(server string, connections int) *redis.Pool {
	return &redis.Pool{
		MaxIdle:     connections,
		MaxActive:   connections,
		IdleTimeout: 240 * time.Second,
		Dial: func() (redis.Conn, error) {
			c, err := redis.Dial("tcp", server)
			if err != nil {
				return nil, err
			}
			// if _, err := c.Do("AUTH", password); err != nil {
			// 	c.Close()
			// 	return nil, err
			// }
			return c, err
		},
		TestOnBorrow: func(c redis.Conn, t time.Time) error {
			if time.Since(t) < time.Minute {
				return nil
			}
			_, err := c.Do("PING")
			return err
		},
	}
}

func startHTTPServer(redisServer string, connections int, httpPort int) {
	pool := newPool(redisServer, connections)
	pools[int64(httpPort)] = pool

	if err := fasthttp.ListenAndServe(":"+strconv.Itoa(httpPort), requestHandler); err != nil {
		log.Fatalf("Error in ListenAndServe: %s", err)
	}
}

func requestHandler(ctx *fasthttp.RequestCtx) {
	addressParts := strings.Split(ctx.LocalAddr().String(), ":")
	port, _ := strconv.ParseInt(addressParts[1], 10, 32)
	pool := pools[port]
	conn := pool.Get()

	key := ctx.Request.Header.Peek("key")
	value := ctx.Request.Header.Peek("value")

	_, err := conn.Do("SET", key, value)
	if err != nil {
		ctx.Response.SetStatusCode(500)
		fmt.Println(err)
	} else {
		ctx.Response.SetStatusCode(200)
	}
	conn.Close()
}
