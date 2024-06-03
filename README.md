# Golang Telegram Bot Finite State Machine

It's a wrapper around [Telegram Bot API Bindings](https://github.com/go-telegram-bot-api/telegram-bot-api). This 
library provides useful scaffolds to build bots which functionality is based on direct messages (DM) communication. 
It is not suitable for telegram groups bots. 

DM bot communication resembles finite state machine (fsm): every user 
interaction may either transit bot into another state or leave state the same (transit to the same state). Every 
transition is accompanied by a certain message from the bot. Moreover, to make any state switching more useful 
and meaningful, each transition may change underlying payload data.