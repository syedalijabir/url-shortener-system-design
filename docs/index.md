---
title: "URL Shortener System Design"
description: "A scalable URL shortener architecture with reverse proxy, gateway, load balancing, caching, database tier, and Docker deployment."
---

# URL Shortener System Design

Welcome to the documentation site for the **URL Shortener System Design** project.  
This project explains how to design and implement a scalable URL shortener using:

- Reverse proxies  
- API gateway  
- Microservices  
- Load balancing  
- Caching with Redis  
- Storage with Postgres  
- Docker containerization  

It follows modern system design best practices and includes diagrams, explanations, and a full Docker-based implementation.

---

## 📘 What You Will Learn

- How a URL shortener works internally  
- How to design scalable microservices  
- Why use reverse proxies, gateways, and internal load balancers  
- How caching reduces database load  
- How to containerize a distributed system using Docker  

---

## 📐 Architecture Overview

![URL shortener high-level architecture](./diagrams/top-level.png)

The system is divided into three main layers:

1. **Reverse Proxy Layer** — Entry point for users  
2. **Gateway Layer** — Converts external HTTP/S into internal gRPC calls  
3. **Service Layer** — URL, storage, and cache services  
4. **Data Layer** — Postgres + Redis  

Read the full breakdown in the [Architecture](architecture.md) page.

---

## 🚀 Implementation Overview

![Docker architecture diagram](./diagrams/docker-architecture.png)

Each component is deployed as a Docker container:

- NGINX for reverse proxy & internal load balancers  
- Go for URL logic, storage, and cache  
- PostgreSQL container for persistence  
- Redis for fast lookup  

Read more in the [Implementation](implementation.md) page.

---

## ⭐ GitHub Repository

👉 [View the source code on GitHub](../README.md)

---

## 🧭 Navigation

- [Architecture](architecture.md)  
- [Implementation](implementation.md)  
- [Diagrams](./diagrams/)  

---

## 📄 License

MIT License
