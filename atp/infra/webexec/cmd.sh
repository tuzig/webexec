#!/usr/bin/env bash
/etc/init.d/ssh start
go run . init 
go run . start 
while true
do
    sleep 3
done

