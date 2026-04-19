<script lang="ts" setup>
import { useResize } from "./composables/useResize"
import LeftMode from "./modes/LeftMode.vue"

useResize()
</script>

<template>
  <LeftMode />
</template>
