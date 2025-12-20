---
title: "URL Shortener System Design"
description: "Scalable URL shortener architecture, diagrams, and Docker implementation."
keywords: ["system design", "url shortener", "architecture", "docker", "microservices", "redis", "postgres"]
---

# Implementation (Dockerized)

This project is fully containerized using Docker.

## Reverse Proxy
Implemented as an NGINX container that exposes ports 80/443.

## Gateway
Runs as a Docker container, offloading HTTP/S to gRPC.

## URL-SVC Load Balancing
An internal NGINX container distributes traffic across multiple URL-SVC replicas. In production this done using autoscaling behind a load balancer.

## Storage Service
Multiple STRG-SVC containers sit behind a load balancer and talk to Postgres.

## Cache Service
Multiple CACHE-SVC containers sit behind a load balancer and talk to Redis.

All services communicate via an internal Docker network to ensure isolation and security. For the purpose of this demo we only use a single network.

UI is available at `http://locahost`

![UI View](./diagrams/frontend.png)
