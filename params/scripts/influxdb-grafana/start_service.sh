!#/bin/bash

echo "grafana port: 3000, influxdb port: 8086"
echo "You should mapping two ports!!!"

echo "start grafana service"
service grafana-server start 

echo "start influxdb service"
nohup influxd -config /etc/influxdb/influxdb.conf &
sleep 1
echo "create database"
influx -execute 'create database cnts'
influx -execute 'show databases'
tail -f nohup.out 
