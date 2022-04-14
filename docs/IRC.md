# Twitch IRC Behaviour

This document aims to define what happens during edge cases on the Twitch IRC

## Join normal channel

```
<- JOIN #troykomodo
-> :7tvapp!7tvapp@7tvapp.tmi.twitch.tv JOIN #troykomodo
-> :7tvapp.tmi.twitch.tv 353 7tvapp = #troykomodo :7tvapp
-> :7tvapp.tmi.twitch.tv 366 7tvapp #troykomodo :End of /NAMES list
-> @badge-info=;badges=;color=#00FF7F;display-name=7tvApp;emote-sets=0,300374282,537206155,564265402,592920959,610186276;mod=0;subscriber=0;user-type= :tmi.twitch.tv USERSTATE #troykomodo
-> @emote-only=0;followers-only=-1;r9k=0;rituals=0;room-id=121903137;slow=0;subs-only=0 :tmi.twitch.tv ROOMSTATE #troykomodo
```


## Join banned channel

```
<- JOIN #starjadian
-> @msg-id=msg_channel_suspended :tmi.twitch.tv NOTICE #starjadian :This channel does not exist or has been suspended.
```


## Channel changes name

    Nothing happens, no part the room just becomes invalid you cannot send messages and cannot receive messages. Not sure how to detect this one.


## Join non-existant channel

```
<- JOIN #sadfasdasdasdasdasd
```

    There is no response, so you must implement a timeout here, if you dont get a response in 20s consider the channel non-existant.


## Channel gets banned

    Untested

    What likely happens is you get a PART message and then on reconnect you are trying to join a suspended channel.


## Bot gets banned

```
-> :7tvapp!7tvapp@7tvapp.tmi.twitch.tv PART #troykomodo
<- JOIN #troykomodo
-> @msg-id=msg_banned :tmi.twitch.tv NOTICE #troykomodo :You are permanently banned from talking in troykomodo.
```
