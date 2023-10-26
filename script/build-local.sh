#!/bin/bash
set -e

rm -rf dist/
mkdir dist/

docker build . -t udpfw-tmp:tmp
docker create --name udpfw-tmp udpfw-tmp:tmp
docker cp udpfw-tmp:/opt/udpfw/nodelet dist/nodelet
docker cp udpfw-tmp:/opt/udpfw/dispatch dist/dispatch
docker rm udpfw-tmp
docker rmi udpfw-tmp:tmp
