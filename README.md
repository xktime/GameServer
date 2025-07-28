docker network create my-network
docker pull redis:latest
docker run -itd --name redis --network my-network -p 6379:6379 redis
 
docker pull mongo:latest
docker run -d -p 27017:27017 --network my-network --name mongo mongo

docker build -t gameserver:latest .
docker run -p 3653:3653 -p 3563:3563 --network my-network --name gameserver gameserver:latest