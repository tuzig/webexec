# TMUX Support

webexec has code that runs only when the client requested a command that starts
with `tmux -CC`. This code opens a data channel for each pane and communicates
with the client using json messages over the original data channel.

Each message must include a `version` field, specifying the protocol's version
and a `time` - a float specifying how may seconds passed since 1/1/1970.
In addition, messages include the fields required by specific commands:

## Server Messages

### Layout Update

The layout update is sent on two ocaasions:
- When the server discovers the sessions' layout has changed
- When the client requests a command that changes the layout,
the code quieries tmux for the lates windows and panes and sends the new layout

The message below descripes a session with two windows, the first has a single
pane and the second a three pane layout that looks like ` |-`.


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
        "t_alert": 0,
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

### Toogle Zoom

When toggling the zoom the specified pane is becoming the active pane.

```json
{
    "version": 1,
    "time": 1589355555.147976,
    "toggle_zoom": "45"
}
```

### Resize Pane

Resizing a pane can be done by passing the `up`, `down`, `left` or `right` field
as part of the `resize_pane` field and setting it to the number of rows/cols.
For example, to grow a pane by 3 rows: 

```json
{
    "version": 1,
    "time": 1589355555.147976,
    "resize_pane": {
        "id": "45",
        "down": 3
    }
}
```

You can mix "down" with "right" or "left" to grow the pane in two directions.

### Break Pane

The `active` field can be used to specify whether the new window should be
docused (defaul is true). 

```json
{
    "version": 1,
    "time": 1589355555.147976,
    "break_pane": {
        "id": "A window",
        "active": false,
        "down": 3,
        "command": "htop",
        "env": {
            "varA": "valueA"
        }
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

### Kill pane

```json
{
    "version": 1,
    "time": 1589355555.147976,
    "kill-pane": "%XY"
}
```

### New Window

The `active` field can be used to specify whether the new window should be
focused (default is true). The `env` field for setting environment variables. 
```json
{
    "version": 1,
    "time": 1589355555.147976,
    "new_window": {
        "name": "A window",
        "active": false,
        "command": "htop",
        "env": {
            "varA": "valueA"
        }
    }
}
```

### Select Window 

This message changes the active window.


```json
{
    "version": 1,
    "time": 1589355555.147976,
    "select-window": "@XY"
}
```

### Rotate Window 

This message rotates the panes inside a window.


```json
{
    "version": 1,
    "time": 1589355555.147976,
    "rotate-window": "@XY"
}
```

### Select Layout 

The user can change the way panes are laid out on the current window.
`layou_name` can be one of: `even-horizontal`, `even-vertical`,
`main-horizontal`, `main-vertical` or `tiled`.


```json
{
    "version": 1,
    "time": 1589355555.147976,
    "refresh-client": {
        "window_id": "@XY",
        "layout": "<layout_name>"
    }
}
```

### rename Window

```json
{
    "version": 1,
    "time": 1589355555.147976,
    "rename-window": {
        "id": "@XY",
        "name": "Window's name"
    }
}
```

### Kill Window

```json
{
    "version": 1,
    "time": 1589355555.147976,
    "kill-window": "@XY"
}
```

### Refresh Client

When the user changes the size of the its windows it send a message:

```json
{
    "version": 1,
    "time": 1589355555.147976,
    "refresh-client": {
        "sx": 80,
        "sy": 24
    }
}
```

### Run a tmux command

This message executes a tmux command

```json
{
    "version": 1,
    "time": 1589355555.147976,
    "command": "list-panes -a"
}
```

Help Needed
-----------

I need help adding a plugin architecture and refactoring tmux support
as a plugin. It's probably worth doing only if you have an idea for a plugin
you want to develop. Issue #XYZ.
