<script lang="ts" setup>
import screenfull from "screenfull"

interface Props {
  /** Element to fullscreen, defaults to html */
  element?: string
  /** Tooltip text for entering fullscreen */
  openTips?: string
  /** Tooltip text for exiting fullscreen */
  exitTips?: string
  /** Whether to apply fullscreen only to the content area */
  content?: boolean
}

const { element = "html", openTips = "Fullscreen", exitTips = "Exit Fullscreen", content = false } = defineProps<Props>()

const CONTENT_LARGE = "content-large"

const CONTENT_FULL = "content-full"

const classList = document.body.classList

// #region Fullscreen
const isEnabled = screenfull.isEnabled

const isFullscreen = ref<boolean>(false)

const fullscreenTips = computed(() => (isFullscreen.value ? exitTips : openTips))

const fullscreenSvgName = computed(() => (isFullscreen.value ? "fullscreen-exit" : "fullscreen"))

function handleFullscreenClick() {
  const dom = document.querySelector(element) || undefined
  isEnabled ? screenfull.toggle(dom) : ElMessage.warning("Your browser does not support fullscreen")
}

function handleFullscreenChange() {
  isFullscreen.value = screenfull.isFullscreen
  // Clear related classes when exiting fullscreen
  isFullscreen.value || classList.remove(CONTENT_LARGE, CONTENT_FULL)
}

watchEffect(() => {
  if (isEnabled) {
    // Automatically runs when component is mounted
    screenfull.on("change", handleFullscreenChange)
    // Automatically runs when component is unmounted
    onWatcherCleanup(() => {
      screenfull.off("change", handleFullscreenChange)
    })
  }
})
// #endregion

// #region Content area
const isContentLarge = ref<boolean>(false)

const contentLargeTips = computed(() => (isContentLarge.value ? "Content Restore" : "Content Expand"))

const contentLargeSvgName = computed(() => (isContentLarge.value ? "fullscreen-exit" : "fullscreen"))

function handleContentLargeClick() {
  isContentLarge.value = !isContentLarge.value
  // Hide unnecessary components when content area is expanded
  classList.toggle(CONTENT_LARGE, isContentLarge.value)
}

function handleContentFullClick() {
  // Cancel content area expand
  isContentLarge.value && handleContentLargeClick()
  // Hide unnecessary components when content area is fullscreen
  classList.add(CONTENT_FULL)
  // Enter fullscreen
  handleFullscreenClick()
}
// #endregion
</script>

<template>
  <div>
    <!-- Fullscreen -->
    <el-tooltip v-if="!content" effect="dark" :content="fullscreenTips" placement="bottom">
      <SvgIcon :name="fullscreenSvgName" @click="handleFullscreenClick" class="svg-icon" />
    </el-tooltip>
    <!-- Content area -->
    <el-dropdown v-else :disabled="isFullscreen">
      <SvgIcon :name="contentLargeSvgName" class="svg-icon" />
      <template #dropdown>
        <el-dropdown-menu>
          <!-- Content area expand -->
          <el-dropdown-item @click="handleContentLargeClick">
            {{ contentLargeTips }}
          </el-dropdown-item>
          <!-- Content area fullscreen -->
          <el-dropdown-item @click="handleContentFullClick">
            Content Fullscreen
          </el-dropdown-item>
        </el-dropdown-menu>
      </template>
    </el-dropdown>
  </div>
</template>

<style lang="scss" scoped>
.svg-icon {
  font-size: 20px;
  &:focus {
    outline: none;
  }
}
</style>
