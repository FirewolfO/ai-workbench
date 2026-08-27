<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { CircleCheck, Collection, Document, DocumentCopy, Download, Files, Folder, MagicStick, Picture, Scissor, Tickets, UploadFilled } from '@element-plus/icons-vue'
import { ElMessage } from 'element-plus'
import { apiMessage, workbenchApi } from '@/api'
import type { FileTool, FileToolResult, FileToolRunOptions } from '@/types'

const icons: Record<string, unknown> = { Document, DocumentCopy, Download, Files, Folder, MagicStick, Picture, Scissor, Tickets, Collection }
const loading = ref(true)
const running = ref(false)
const progress = ref(0)
const tools = ref<FileTool[]>([])
const selectedID = ref('')
const files = ref<File[]>([])
const fileInput = ref<HTMLInputElement | null>(null)
const result = ref<FileToolResult | null>(null)
const options = reactive<Record<string, string | number>>({})
const selected = computed(() => tools.value.find(tool => tool.id === selectedID.value) || null)
const groups = computed(() => {
  const grouped = new Map<string, FileTool[]>()
  for (const tool of tools.value) grouped.set(tool.category, [...(grouped.get(tool.category) || []), tool])
  return [...grouped.entries()].map(([name, items]) => ({ name, items }))
})
const totalSize = computed(() => files.value.reduce((total, file) => total + file.size, 0))
const canRun = computed(() => Boolean(selected.value?.available && files.value.length >= (selected.value?.minFiles || 1) && files.value.length <= (selected.value?.maxFiles || 1) && totalSize.value <= 64 * 1024 * 1024 && !running.value))

function formatSize(bytes: number) {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${Math.ceil(bytes / 1024)} KB`
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`
}

function chooseTool(tool: FileTool, scroll = true) {
  if (!tool.available) {
    ElMessage.warning(tool.unavailableReason || '该工具暂不可用')
    return
  }
  selectedID.value = tool.id
  files.value = []
  result.value = null
  progress.value = 0
  for (const key of Object.keys(options)) delete options[key]
  for (const option of tool.options || []) options[option.id] = option.default ?? ''
  if (scroll) requestAnimationFrame(() => document.querySelector('.tool-runner')?.scrollIntoView({ behavior: 'smooth', block: 'start' }))
}

function openPicker() { fileInput.value?.click() }
function selectFiles(event: Event) {
  const input = event.target as HTMLInputElement
  addFiles(Array.from(input.files || []))
  input.value = ''
}
function addFiles(incoming: File[]) {
  const tool = selected.value
  if (!tool || !incoming.length) return
  const next = tool.multiple ? [...files.value, ...incoming] : incoming.slice(0, 1)
  files.value = next.slice(0, tool.maxFiles)
  result.value = null
  if (next.length > tool.maxFiles) ElMessage.warning(`这个工具最多选择 ${tool.maxFiles} 个文件`)
  if (totalSize.value > 64 * 1024 * 1024) ElMessage.warning('单次处理的文件总大小不能超过 64 MiB')
}
function dropFiles(event: DragEvent) { addFiles(Array.from(event.dataTransfer?.files || [])) }
function removeFile(index: number) { files.value.splice(index, 1); result.value = null }

async function run() {
  if (!selected.value || !canRun.value) return
  running.value = true
  progress.value = 0
  result.value = null
  try {
    const runOptions: FileToolRunOptions = {
      pageRange: String(options.pageRange ?? ''),
      quality: String(options.quality ?? ''),
      imageFormat: String(options.imageFormat ?? ''),
      maxWidth: Number(options.maxWidth || 0),
    }
    result.value = await workbenchApi.runFileTool(selected.value.id, files.value, runOptions, value => { progress.value = value })
    progress.value = 100
    ElMessage.success('文件处理完成')
  } catch (error) {
    ElMessage.error(apiMessage(error, '文件处理失败，请检查文件后重试'))
  } finally {
    running.value = false
  }
}

onMounted(async () => {
  try {
    tools.value = await workbenchApi.fileTools()
    const first = tools.value.find(tool => tool.available)
    if (first) chooseTool(first, false)
  } catch (error) {
    ElMessage.error(apiMessage(error, '工具列表加载失败'))
  } finally {
    loading.value = false
  }
})
</script>

<template>
  <div v-loading="loading" class="tools-page">
    <header class="tools-heading">
      <div><span>LOCAL FILE TOOLKIT</span><h2>实用工具</h2><p>文档、PDF、图片和文件整理都在工作台本地服务中完成，不消耗模型额度。</p></div>
      <div class="privacy-chip"><el-icon><CircleCheck /></el-icon><span><strong>文件不发给 AI</strong><small>结果 7 天后自动删除</small></span></div>
    </header>

    <section v-for="group in groups" :key="group.name" class="tool-group">
      <h3>{{ group.name }}</h3>
      <div class="tool-grid">
        <button v-for="tool in group.items" :key="tool.id" type="button" class="tool-card" :class="{ active: selectedID === tool.id, unavailable: !tool.available }" @click="chooseTool(tool)">
          <span class="tool-icon"><el-icon><component :is="icons[tool.icon] || Files" /></el-icon></span>
          <span><strong>{{ tool.name }}</strong><small>{{ tool.description }}</small><em v-if="!tool.available">{{ tool.unavailableReason }}</em></span>
        </button>
      </div>
    </section>

    <section v-if="selected" class="tool-runner">
      <header><span class="tool-icon large"><el-icon><component :is="icons[selected.icon] || Files" /></el-icon></span><div><small>当前工具</small><h3>{{ selected.name }}</h3><p>{{ selected.description }}</p></div></header>
      <button class="file-drop" type="button" @click="openPicker" @dragover.prevent @drop.prevent="dropFiles">
        <el-icon><UploadFilled /></el-icon><strong>{{ files.length ? '继续选择文件' : '选择文件' }}</strong>
        <span>{{ selected.multiple ? `可选择 ${selected.minFiles}–${selected.maxFiles} 个文件` : '选择 1 个文件' }} · 单次总计不超过 64 MiB</span>
      </button>
      <input ref="fileInput" class="file-input" type="file" :accept="selected.accept" :multiple="selected.multiple" @change="selectFiles" />

      <div v-if="files.length" class="selected-files">
        <article v-for="(file, index) in files" :key="`${file.name}-${file.lastModified}-${index}`"><span><strong>{{ file.name }}</strong><small>{{ formatSize(file.size) }}</small></span><el-button text aria-label="移除文件" @click="removeFile(index)">移除</el-button></article>
        <footer><span>已选择 {{ files.length }} 个 · {{ formatSize(totalSize) }}</span><el-button text @click="files = []">清空</el-button></footer>
      </div>

      <div v-if="selected.options?.length" class="tool-options">
        <label v-for="option in selected.options" :key="option.id"><span>{{ option.label }}</span>
          <el-select v-if="option.type === 'select'" v-model="options[option.id]"><el-option v-for="choice in option.choices" :key="choice.value" :label="choice.label" :value="choice.value" /></el-select>
          <el-input-number v-else-if="option.type === 'number'" v-model="options[option.id]" :min="option.id === 'quality' ? 20 : 0" :max="option.id === 'quality' ? 100 : 10000" controls-position="right" />
          <el-input v-else v-model="options[option.id]" :placeholder="option.placeholder" />
        </label>
      </div>

      <el-progress v-if="running" class="tool-progress" :percentage="progress" :indeterminate="progress >= 100" :duration="2" />
      <el-button class="run-tool" type="primary" size="large" :loading="running" :disabled="!canRun" @click="run">{{ running ? (progress < 100 ? '正在上传…' : '正在处理…') : '开始处理' }}</el-button>
      <p v-if="files.length && files.length < selected.minFiles" class="runner-hint">还需要选择至少 {{ selected.minFiles - files.length }} 个文件</p>

      <div v-if="result" class="tool-result" aria-live="polite">
        <el-icon><CircleCheck /></el-icon><div><strong>{{ result.summary }}</strong><span>{{ result.name }} · {{ formatSize(result.size) }}</span><small>下载链接有效至 {{ new Date(result.expiresAt).toLocaleString('zh-CN') }}</small></div>
        <el-button tag="a" type="primary" :href="result.downloadUrl" :download="result.name" :icon="Download">下载结果</el-button>
      </div>
    </section>
  </div>
</template>

<style scoped>
.tools-page{width:min(1180px,calc(100% - 48px));margin:0 auto;padding:32px 0 60px}.tools-heading{display:flex;align-items:flex-end;justify-content:space-between;gap:28px;margin-bottom:30px}.tools-heading>div:first-child>span{color:#176b55;font-size:10px;font-weight:800}.tools-heading h2{margin:7px 0 6px;font-size:27px}.tools-heading p{margin:0;color:#697571;font-size:13px}.privacy-chip{display:flex;align-items:center;gap:9px;min-width:210px;padding:11px 13px;border:1px solid #bdd7d0;border-radius:7px;color:#176b55;background:#edf7f3}.privacy-chip>.el-icon{font-size:20px}.privacy-chip strong,.privacy-chip small{display:block}.privacy-chip strong{font-size:12px}.privacy-chip small{margin-top:3px;color:#648078;font-size:10px}.tool-group{margin-top:25px}.tool-group>h3{margin:0 0 11px;color:#52605c;font-size:13px}.tool-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:10px}.tool-card{display:grid;grid-template-columns:42px minmax(0,1fr);align-items:start;gap:11px;min-height:108px;padding:15px;border:1px solid #dce4e1;border-radius:7px;color:inherit;text-align:left;background:#fff;cursor:pointer;transition:border-color .16s,transform .16s,box-shadow .16s}.tool-card:hover{border-color:#8db6aa;transform:translateY(-1px);box-shadow:0 6px 18px rgba(24,40,36,.06)}.tool-card.active{border-color:#176b55;box-shadow:0 0 0 1px #176b55}.tool-card.unavailable{opacity:.58;cursor:not-allowed}.tool-icon{display:grid;place-items:center;width:42px;height:42px;border-radius:7px;color:#176b55;background:#e8f2ef;font-size:20px}.tool-icon.large{width:48px;height:48px;font-size:23px}.tool-card strong,.tool-card small,.tool-card em{display:block}.tool-card strong{margin:2px 0 6px;font-size:14px}.tool-card small{color:#6f7b77;font-size:11px;line-height:1.55}.tool-card em{margin-top:5px;color:#a35a35;font-size:10px;font-style:normal}.tool-runner{scroll-margin-top:78px;margin-top:32px;padding:24px;border:1px solid #d8e1de;border-radius:9px;background:#fff;box-shadow:0 10px 32px rgba(24,40,36,.05)}.tool-runner>header{display:flex;align-items:center;gap:13px}.tool-runner>header small{color:#7b8783;font-size:10px}.tool-runner>header h3{margin:3px 0 2px;font-size:18px}.tool-runner>header p{margin:0;color:#6d7975;font-size:11px}.file-drop{display:grid;place-items:center;width:100%;min-height:144px;margin-top:20px;padding:18px;border:1px dashed #9db8b0;border-radius:7px;color:#176b55;background:#f8fbfa;cursor:pointer}.file-drop:hover{border-color:#176b55;background:#f0f7f5}.file-drop>.el-icon{font-size:27px}.file-drop strong{margin:10px 0 5px;font-size:14px}.file-drop span{color:#77837f;font-size:10px}.selected-files{margin-top:12px;border:1px solid #e0e6e4;border-radius:7px;overflow:hidden}.selected-files article,.selected-files footer{display:flex;align-items:center;justify-content:space-between;gap:10px;min-height:46px;padding:7px 12px;border-bottom:1px solid #edf0ef}.selected-files article>span{min-width:0}.selected-files strong,.selected-files small{display:block}.selected-files strong{overflow:hidden;font-size:12px;text-overflow:ellipsis;white-space:nowrap}.selected-files small{margin-top:2px;color:#89928f;font-size:9px}.selected-files footer{min-height:40px;border-bottom:0;color:#6f7b77;background:#fafbfa;font-size:10px}.tool-options{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:12px;margin-top:16px}.tool-options label>span{display:block;margin-bottom:5px;color:#586561;font-size:11px}.tool-options :deep(.el-select),.tool-options :deep(.el-input-number){width:100%}.tool-progress{margin-top:18px}.run-tool{width:100%;margin-top:18px}.runner-hint{margin:8px 0 0;color:#a16e25;font-size:10px;text-align:center}.tool-result{display:grid;grid-template-columns:36px minmax(0,1fr) auto;align-items:center;gap:12px;margin-top:18px;padding:15px;border:1px solid #a8cfc4;border-radius:7px;color:#176b55;background:#edf7f3}.tool-result>.el-icon{font-size:27px}.tool-result strong,.tool-result span,.tool-result small{display:block}.tool-result strong{font-size:13px}.tool-result span{margin-top:4px;color:#4d655e;font-size:11px}.tool-result small{margin-top:3px;color:#789088;font-size:9px}@media(max-width:900px){.tool-grid{grid-template-columns:repeat(2,minmax(0,1fr))}}@media(max-width:600px){.tools-page{width:calc(100% - 24px);padding:20px 0 36px}.tools-heading{align-items:stretch;flex-direction:column;gap:15px}.tools-heading h2{font-size:23px}.privacy-chip{width:100%}.tool-group{margin-top:21px}.tool-grid{grid-template-columns:1fr}.tool-card{min-height:94px;padding:13px}.tool-runner{scroll-margin-top:66px;margin-top:24px;padding:15px}.file-drop{min-height:126px}.tool-options{grid-template-columns:1fr}.tool-result{grid-template-columns:32px minmax(0,1fr)}.tool-result>.el-button{grid-column:1/-1;width:100%}}
</style>
