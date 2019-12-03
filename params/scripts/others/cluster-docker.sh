#/bin/bash 
docker-compose down 

#docker-compose run cnts-cluster-central
docker-compose run cnts-server
#docker-compose up --scale cnts-server=2
