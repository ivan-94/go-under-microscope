package main

import (
	"fmt"
	"io/ioutil"
	"net/http"

	"./ws"
)

func main() {
	http.Handle("/ws", ws.Handler(func(conn *ws.Conn) {
		data, err := ioutil.ReadAll(conn)
		if err != nil {
			fmt.Println("接收失败", err.Error())
		} else {
			fmt.Println(string(data))
			_, err := conn.Write([]byte("hello world too"))
			if err != nil {
				fmt.Println("发送失败", err.Error())
			}
		}
	}))
	http.Handle("/public/", http.StripPrefix("/public/", http.FileServer(http.Dir("./client"))))
	http.ListenAndServe(":4080", nil)
}
