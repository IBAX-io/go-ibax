#!/bin/bash
set -e -x

cd /mnt/ibax

if [ ! -f "/mnt/ibax/data/config.toml" ]; then
  /mnt/ibax/go-ibax config \
    --tls="$TLS_ENABLE" \
    --tls-cert="$TLS_CERT" \
    --tls-key="$TLS_KEY" \
    --mbs="$HTTPSERVERMAXBODYSIZE" \
    --mpgt="$MAXPAGEGENERATIONTIME" \
    --nodesAddr="$NODES_ADDR" \
    --tcpHost="$TCPSERVER_HOST" \
    --tcpPort="$TCPSERVER_PORT" \
    --httpHost="$HTTP_HOST" \
    --httpPort="$HTTP_PORT" \
    --dbHost="$DB_HOST" \
    --centUrl="$CENTRIFUGO_URL" \
    --cryptoer="$CRYPTOSETTINGS_CRYPTOER" \
    --hasher="$CRYPTOSETTINGS_HASHER"
fi

if [ ! -f "/mnt/ibax/data/NodePublicKey" ]; then
  /mnt/ibax/go-ibax generateKeys
fi

if [ 0"$NODES_ADDR" = "0" ]; then
  if [ ! -f "/mnt/ibax/data/1block" ]; then
    /mnt/ibax/go-ibax generateFirstBlock --test true
  fi
fi

if [ ! -f "/mnt/ibax/data/initDatabase.txt" ]; then
  sleep 3
  /mnt/ibax/go-ibax initDatabase
  touch /mnt/ibax/data/initDatabase.txt
  echo initDatabase >> /mnt/ibax/data/initDatabase.txt
  sleep 1
fi

/mnt/ibax/go-ibax start




