# Route Disabled Feature — Frontend Requirements

## Overview

Routes support a `disabled` field. When a route is disabled, incoming requests using that route key are treated as if the key does not exist — they fall through to the default route chain (protocol-specific default first, then global default).

This allows temporary route deactivation without deleting or modifying the route configuration.

## API Changes

### List Routes — `GET /api/admin/routes`

Response now includes a `disabled` boolean field on each route item.

```json
{
  "data": [
    {
      "key": "gw-deepseek",
      "provider": "deepseek",
      "default_model": "deepseek-chat",
      "disabled": false,
      "scene_map": {},
      "model_map": {},
      "long_context_threshold": 0
    },
    {
      "key": "gw-disabled-route",
      "provider": "minimax",
      "default_model": "default-model",
      "disabled": true,
      "scene_map": {},
      "model_map": {},
      "long_context_threshold": 0
    }
  ]
}
```

### Update Route — `PUT /api/admin/routes/:key`

Accepts a `disabled` field (optional, `*bool`). Send `true` to disable, `false` to re-enable.

**Toggle example:**

```json
PUT /api/admin/routes/gw-deepseek
{
  "disabled": true
}
```

Response:

```json
{
  "data": { "key": "gw-deepseek" }
}
```

### Create Route — `POST /api/admin/routes`

Accepts an optional `disabled` field (bool, defaults to `false`).

```json
{
  "key": "gw-test",
  "provider": "deepseek",
  "default_model": "deepseek-chat",
  "disabled": true
}
```

## Functional Requirements

1. **Route list page**: Show the `disabled` state for each route — visually distinguish disabled routes from active ones (e.g. grayed out, badge, toggle switch).

2. **Toggle disable/enable**: Provide a quick toggle action (switch or button) on each route row. Calling `PUT /api/admin/routes/:key` with `{"disabled": true/false}`.

3. **Create route form**: Include a `disabled` toggle in the route creation form. Default is off (not disabled). Users may pre-disable a route during creation.

4. **Disabled route indication**: Disabled routes should be visually distinct in the route list so users can quickly identify which routes are inactive at a glance.
