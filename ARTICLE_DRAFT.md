# Building Talos: My First Open Source Deployment Platform

Hola amigos,

This is my first open source project, and it started from a very practical frustration: deploying side projects should not feel like repeating the same VPS setup ritual forever.

The funny version of the story is that I built this with the assistance of Bruno Fernandes. The real version is less glamorous: I built it with help from MiMo-V2.5-Pro, a lot of curiosity, and the urge to make my own life easier.

The idea clicked after I saw people talking about Xiaomi giving away free tokens. Around the same time, I kept running into the same pain with side projects. Every time I wanted to ship something, I would rent a cheap VPS, install the same dependencies again, wire up Docker again, configure a reverse proxy again, and redo all the small deployment chores that are never hard individually, but become exhausting when repeated over and over.

That made me ask a simple question:

> What if I build a tool that gives me a smoother deployment workflow on my own VPS?

That question became **Project Talos**.

## What Talos is

Talos is a self-hosted deployment platform for Dockerized applications on a **single VPS**. The goal is not to compete with Kubernetes or become a giant platform-as-a-service. The goal is much narrower:

- deploy applications to one VPS easily
- manage app routing in one place
- keep the workflow simple enough for solo developers and small teams
- avoid repeating the same server setup work for every side project

If Vercel and Heroku made deployment feel approachable, Talos is my attempt to bring some of that experience into a self-hosted environment that I control myself.

## Why I built it

I did not build Talos because the world needed another massive infrastructure platform. I built it because I had a repeated problem:

- I like building side projects
- I usually deploy them to cheap VPS instances
- I do not like redoing server setup every single time
- I wanted a control plane that stays lightweight, understandable, and mine

So Talos is really a product of developer laziness in the best sense: automate the boring parts, standardize the flow, and reduce setup fatigue.

## Why the name "Talos"

I took the name from Greek mythology. Talos was a giant bronze guardian, and that felt fitting for a project that sits in front of deployed apps and helps manage, route, and protect them.

The name also gives the project a bit more personality than calling it something like `deploy-manager-v1`.

## Why Go and HTMX

A fair question is: why Go and HTMX?

Also, why not Java, even though Aidit said *"Java is key"*.

My answer is simple: I wanted the stack to stay lightweight and direct.

### Why Go

I chose Go because:

- it produces a single binary that is easy to ship
- it is lightweight for server-side tools
- it keeps the backend simple and readable
- it is a strong fit for infrastructure-style software

Talos is not a giant enterprise platform. It is an opinionated deployment tool. For that kind of product, Go feels like a very natural choice.

### Why HTMX

I used HTMX because I wanted an interactive web UI without building a heavy frontend stack. Talos is mostly an operational dashboard, not a highly dynamic consumer application, so server-rendered HTML plus incremental interactivity felt like the right tradeoff.

That let me focus more on product behavior and deployment flow, and less on frontend complexity.

## What Talos currently does

Right now, Talos is focused on a clear scope instead of trying to solve everything:

- runs as a self-hosted control plane on one VPS
- manages Dockerized applications
- stores state in SQLite
- uses Traefik for routing
- supports GitHub-based deployment flow
- can receive signed deploy webhooks
- tracks deployment history and status
- supports rollback to the previous successful deploy
- provisions backing services like PostgreSQL, MySQL, Redis, and Garage
- encrypts service credentials at rest

That scope matters a lot to me. I would rather build a smaller tool that is coherent and reliable than a wider tool that becomes confusing too early.

## The product philosophy

One thing I want people to understand about Talos is that it is intentionally opinionated.

Talos is built around a few principles:

- one obvious happy path
- reliability before breadth
- opinionated defaults over feature sprawl
- one VPS is a feature, not a limitation to hide

A lot of developer tools become harder to use because they try to support every possible workflow. Talos goes in the opposite direction. I want it to be the tool you can understand quickly, install on your server, and use without needing a 40-page setup guide.

## How Talos works at a high level

The architecture is straightforward:

```text
Browser -> Talos UI/API -> Docker Engine -> App Containers
                      -> Traefik -> Public traffic
```

The browser talks to Talos.
Talos manages app state, deployment records, and service definitions.
Docker runs the actual containers.
Traefik handles routing for domains and public access.

This matters because one of my goals with Talos is to keep the architecture understandable. If I come back to this project months later, I want to be able to reason about it quickly.

## The real problem Talos tries to solve

The main problem is not "how do I deploy containers?"

Docker already helps with that.

The real problem is this:

- how do I make repeated deployments boring and consistent?
- how do I avoid manually wiring reverse proxies every time?
- how do I manage apps and backing services from one place?
- how do I make a cheap VPS feel less disposable and more like a reusable platform?

Talos is my answer to those questions.

## What makes this project interesting to me

This project is special to me for two reasons.

First, it is my first open source project, so it represents a shift from only building for myself to building in public.

Second, it is not a toy idea. It comes from a real workflow problem I keep facing. That gives it a stronger foundation than "I built this because the tech stack looked fun."

## What Talos is not

It is also important to explain the boundaries clearly.

Talos is not:

- a Kubernetes replacement
- a multi-node orchestrator
- a full Heroku clone
- a general CI/CD platform
- a multi-tenant hosting platform

It is a focused deployment platform for people who want something simpler and more self-hosted than mainstream managed platforms.

## Challenges and tradeoffs

If you want the article to feel honest and technical, add a short section about tradeoffs.

For example:

- keeping the scope narrow is a deliberate product decision
- single-VPS simplicity means there are scaling limits
- self-hosting gives control, but also gives responsibility
- opinionated workflows are easier to use, but less flexible

Readers usually trust a project more when the author can explain both the strengths and the boundaries.

## What I learned from building it

This is a good section to add because people like reading build stories, not just feature lists.

Some useful points you can include:

- building a product is often about choosing what not to support
- simple developer experience takes real design discipline
- infrastructure tools need clear operational boundaries
- shipping a narrow useful tool is better than designing an endless roadmap
- open source feels different when real users might depend on your decisions

## What comes next

You can end the article with the roadmap direction.

Some grounded examples:

- improving the deployment flow
- hardening the bootstrap and installation experience
- making logs and observability better
- improving GitHub integration
- refining rollback and recovery behavior
- polishing the UI and operator workflow

That gives readers a reason to follow the project without making promises that are too broad.

## A stronger closing paragraph

If you want a cleaner ending, you can use something like this:

> Talos started as a way to reduce friction in my own side-project workflow, but it has become something more interesting: a small, opinionated self-hosted platform that reflects how I think deployment tools should feel. It is still early, but the goal is clear: make deploying to a VPS simpler, more repeatable, and less annoying.

---

## Recommended Context To Add

If you want this article to be stronger, add these details explicitly:

### 1. Explain the exact pain before Talos

Write 1 short paragraph about your old workflow:

- buy cheap VPS
- install Docker
- configure reverse proxy
- wire domains
- redeploy manually
- repeat everything for the next project

This gives the article a stronger before-and-after story.

### 2. Explain who Talos is for

Be specific:

- indie hackers
- solo developers
- small teams
- people deploying Docker apps to one VPS

If you do not name the audience, readers may assume the project is trying to be a generic platform for everyone.

### 3. Explain why single VPS is intentional

This is important.

Do not present "single VPS" like a temporary weakness. Present it as a design choice:

- simpler mental model
- lower operational complexity
- easier to maintain
- better fit for side projects

### 4. Add one concrete deployment example

For example:

1. Create app in Talos
2. Connect repository
3. Push code
4. GitHub workflow builds image
5. Talos receives webhook
6. Talos deploys container and updates routing

Concrete flows make infrastructure products easier to understand.

### 5. Mention the current stack plainly

You should name the technologies because people reading open source build stories usually want to know the implementation choices quickly:

- Go
- HTMX
- SQLite
- Docker Engine API
- Traefik
- GitHub webhook flow

### 6. Include limitations honestly

A short honest line helps:

> Talos is currently built for single-server deployments and intentionally avoids the complexity of multi-node orchestration.

That sentence sets expectations well.

### 7. Add screenshots or architecture diagram

If you publish this on GitHub, Dev.to, Hashnode, or your blog, screenshots will help a lot:

- dashboard
- app creation page
- deploy history
- service provisioning page

One small diagram of the deployment flow would also make the article stronger.

---

## Shorter Intro Version

If you want a shorter, more casual opening that still sounds polished, use this:

> Hola amigos, this is my first open source project. I built it because I got tired of repeating the same VPS setup every time I wanted to deploy a side project. I wanted something simple: a self-hosted platform that makes deployment easier on a single server. Inspired by tools like Vercel and Heroku, I started building Talos, a lightweight deployment platform built with Go and HTMX.

---

## Suggested Article Title Ideas

- Building Talos: My First Open Source Deployment Platform
- Talos: Why I Built a Self-Hosted Deployment Platform for My VPS
- From VPS Setup Fatigue to Talos
- Building My Own Mini Heroku for Side Projects
- Talos: A Lightweight Self-Hosted Deployment Platform in Go

---

## Suggested Structure For Publishing

If you want to turn this into a final blog post, use this order:

1. Personal intro
2. The problem
3. Why I decided to build Talos
4. What Talos does
5. Why I chose Go and HTMX
6. Architecture overview
7. Product philosophy and scope
8. Challenges and tradeoffs
9. What I learned
10. What comes next

That structure will read much better than jumping straight from the joke intro into stack choices.
