#!/bin/bash
apt install -y make

TARBALL="go1.24.3.linux-amd64.tar.gz"
if [ ! -f "$TARBALL" ]; then
    wget https://go.dev/dl/$TARBALL
fi

rm -rf /usr/local/go
tar -C /usr/local -xzf $TARBALL
echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile
export PATH=$PATH:/usr/local/go/bin
go version
make build
