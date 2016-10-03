package main

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/garyburd/redigo/redis"
	"github.com/valyala/fasthttp"
	goredis "gopkg.in/redis.v4"
)

var pools map[int64]*redis.Pool
var clients map[int64]*goredis.Client
var pipelines map[int64]*goredis.Pipeline

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

	client := goredis.NewClient(&goredis.Options{
		Addr:     redisServer,
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	clients[int64(httpPort)] = client
	pipelines[int64(httpPort)] = client.Pipeline()

	if err := fasthttp.ListenAndServe(":"+strconv.Itoa(httpPort), requestHandler2); err != nil {
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

	err := conn.Send("SET", key, value)
	if err != nil {
		ctx.Response.SetStatusCode(500)
		fmt.Println(err)
	} else {
		ctx.Response.SetStatusCode(200)
	}

	if ctx.ConnRequestNum()%uint64(*pipelined) == 0 {
		_, err := conn.Do("")
		if err != nil {
			fmt.Println(err)
		}
	}
	conn.Close()
}

func requestHandler2(ctx *fasthttp.RequestCtx) {
	addressParts := strings.Split(ctx.LocalAddr().String(), ":")
	port, _ := strconv.ParseInt(addressParts[1], 10, 32)
	// client := clients[port]
	pipeline := pipelines[port]

	key := ctx.Request.Header.Peek("key")
	value := ctx.Request.Header.Peek("value")

	// err := client.Set(string(key), value, 0).Err()
	err := pipeline.Set(string(key), value, 0).Err()
	if err != nil {
		ctx.Response.SetStatusCode(500)
		fmt.Println(err)
	} else {
		ctx.Response.SetStatusCode(200)
	}

	if ctx.ConnRequestNum()%uint64(*pipelined) == 0 {
		_, err := pipeline.Exec()

		if err != nil {
			fmt.Println(err)
		}
		// pipeline.Close()
		// pipelines[int64(port)] = client.Pipeline()
	}
}
