import type { Handler } from "mitt"
import type { RouteLocationNormalizedGeneric } from "vue-router"
import mitt from "mitt"

/** Callback function type */
type Callback = (route: RouteLocationNormalizedGeneric) => void

const emitter = mitt()

const key = Symbol("ROUTE_CHANGE")

let latestRoute: RouteLocationNormalizedGeneric

/** Set the latest route info and trigger route change event */
export function setRouteChange(to: RouteLocationNormalizedGeneric) {
  // Emit event
  emitter.emit(key, to)
  // Cache the latest route info
  latestRoute = to
}

/**
 * @name Subscribe to route changes Composable
 * @description 1. Using watch alone to monitor routes wastes rendering performance
 * @description 2. Prefer using this pub/sub pattern for distribution management
 */
export function useRouteListener() {
  // Callback function collection
  const callbackList: Callback[] = []

  // Listen for route changes (optionally execute immediately)
  const listenerRouteChange = (callback: Callback, immediate = false) => {
    // Cache callback function
    callbackList.push(callback)
    // Listen for event
    emitter.on(key, callback as Handler)
    // Optionally execute the callback once immediately
    immediate && latestRoute && callback(latestRoute)
  }

  // Remove route change event listener
  const removeRouteListener = (callback: Callback) => {
    emitter.off(key, callback as Handler)
  }

  // Remove listeners before component is destroyed
  onBeforeUnmount(() => {
    callbackList.forEach(removeRouteListener)
  })

  return { listenerRouteChange, removeRouteListener }
}
