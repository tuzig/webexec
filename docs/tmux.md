# TMUX Support

webexec has code that runs only when the client requested a command that starts
with `tmux -CC`. This code opens a data channel for each pane and communicates
with the client using json messages over the original data channel.

Each message must include a `version` field, specifying the protocol's version
and a `time` - a float specifying how may seconds passed since 1/1/1970.
In addition, messages include the fields required by specific commands:

## Server Messages

### Layout Update

When the server discovers the sessions' layout has changed, it queries tmux
for the sessions windows and panes and sends it back. The message below
descripes a session with two windows, the first has a single pane and the
second a three pane layout that looks like ` |-`.


```json
{ 
    "version": 1,
    "time": 1589355555.147976,
    "layout": [{
        "id": "@41",
        "name": "root",
        "zoomed": false,
        "sx": 190, "sy": 49,
        "xoff": 0, "yoff": 0,
        "active": false,
        "active_clients": 1,
        "active_sessions": 1,
        "last_activity": 1589355444.147976,
        "bigger": false,
        "flags": "*",
        "index": 0,
        "is_last": true,
        "marked": false,
        "silence_flag": 0,
        "stack_index": 0,
        "panes": [{
            "id": "45",
            "sx": 190, "sy": 49,
            "xoff":0, "yoff":0,
            "active": true,
            "current_command": "python",
            "current_path": "/root",
            "exit_status": null,
            "in_mode": false,
            "index": 23,
            "marked": false,
            "last_search_string": "ERROR"
          }]
    }, {
        "id": "@42",
        "name": "demo",
        "zoomed": false,
        "sx": 190,
        "sy": 49,
        "xoff": 0,
        "yoff": 0,
        "active": true,
        "panes": [{
            "id": "46",
            "sx": 95, "sy": 49,
            "xoff": 0, "yoff": 0,
            "active": true
        }, {
            "id": "47",
            "sx": 94, "sy": 24,
            "xoff": 96, "yoff": 0,
            "active": false
        }, {
            "id": "48",
            "sx": 94, "sy": 24,
            "xoff": 96, "yoff": 25,
            "active": false
        }]
    }]
}
```

## Client Messages

### Client Resize

When the user changes the size of the its windows it send a message:

```json
{ 
    "version": 1,
    "time": 1589355555.147976,
    "size": {
        "sx": 80,
        "sy": 24
    }
}
```

### Split Pane

When the user splits a pane the following message is sent: 

```json
{ 
    "version": 1,
    "time": 1589355555.147976,
    "split": {
        "target_pane": 47,
        "type": "topbottom",
        "size": "50%",
        "before_target": false
    }
}
```

Help Needed
-----------

I need help adding a plugin architecture and refactoring tmux support
as a plugin. It's probably worth doing only if you have an idea for a plugin
you want to develop. Issue #XYZ.
