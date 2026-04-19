import { defineConfig, presetWind3 } from "unocss"

export default defineConfig({
  presets: [
    presetWind3({ important: "#app" })
  ],
  shortcuts: {
    "wh-full": "w-full h-full",
    "flex-center": "flex justify-center items-center",
    "flex-x-center": "flex justify-center",
    "flex-y-center": "flex items-center"
  }
})
