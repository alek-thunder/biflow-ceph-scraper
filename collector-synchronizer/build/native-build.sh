#!/bin/bash
home=`dirname $(readlink -f $0)`
root=`readlink -f "$home/.."`
echo "home::: $home root ::: $root"
go build -o "$home/_output/bitflow-pipeline" $@ "$root/../bitflow-pipeline"