# ofisu

Quasi-[MUD][] bot that emulates a physical office.

Currently supports Discord.
Slack is on the roadmap.

## Installation

### Server roles setup

This bot operates by revealing channels to a user as they navigate the office.
Users with "Server Owner" or "Administrator" permissions can see all channels at all times, without exception.
Pretend these people basically have access to a security camera system and can see all rooms of the office at once.

**Recommendation:**
- Create a dummy discord account as the server owner.
- Do not issue "Administrator" access to any of the players.

### Create and invite the bot

1. Create your application at <https://discord.com/developers/applications>.
2. Add a bot user to your application at Settings -> Bot (<https://discord.com/developers/docs/topics/oauth2#bots>)
3. Grant the bot user the "Server Members Intent" at Settings -> Bot -> "Server Members Intent"
4. Add the bot to your server using the OAuth URL Generator at <https://discordapi.com/permissions.html>. **Note:** Currently we recommend just giving the bot "Administrator" permissions. We have [an open issue](https://github.com/austinpray/ofisu/issues/2) on documenting the exact minimum permissions the bot needs to operate, as that would be preferable.

## Running

You can spin up an easy development environment using [docker-compose][].

First you need to create a .env file with the necessary secrets:

```shell
cp .env.example .env
vim .env # replace the default values with the real values

Then build the image and boot the docker-compose services:

```shell
docker-compose build
docker-compose up
```

This will spin up

- An ofisu discord-controller with variables loaded from .env
- A redis database

[mud]: https://en.wikipedia.org/wiki/MUD
[docker-compose]: https://docs.docker.com/compose/
