#!/usr/bin/env bash

echo 'usage:'$0 '[container_pid]'
echo 'example: sudo bash '$0 '6666'

bridge_nic='brg-demo'
bridge_ip='10.10.10.100/24'

veth_host_ip='10.10.10.100'
veth_container_ip='10.10.10.102/24'
veth_host='veth_host1'
veth_container='veth_conta2'
pid=$1 #fetch first argument

#remove previous and add new one
ip link del $bridge_nic
ip link add name $bridge_nic address 12:34:56:a1:b2:c4 type bridge

# add ip to bridge interface
ip addr add $bridge_ip dev $bridge_nic

ip link set dev $bridge_nic up

#add veth peer
ip link add $veth_host type veth peer name $veth_container

ip link set $veth_host up

#attach veth of host side to master network interface
ip link set $veth_host master $bridge_nic

#move container veth to its network namespace
ip link set $veth_container netns $pid


# make process network namespace visible
NETNS=$pid
if [ ! -d /var/run/netns ]; then
    mkdir /var/run/netns
fi
if [ -f /var/run/netns/$NETNS ]; then
    rm -rf /var/run/netns/$NETNS
fi

ln -s /proc/$NETNS/ns/net /var/run/netns/$NETNS
echo "netns: $NETNS"


ip netns exec $NETNS ip addr add $veth_container_ip dev $veth_container

ip netns exec $NETNS ip link set $veth_container up

ip netns exec $NETNS ip route add default via $veth_host_ip dev $veth_container


# make process network namespace invisible
rm -rf /var/run/netns/$NETNS






