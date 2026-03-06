<template>
  <div class="workbench">
    <el-card>
      <template #header>
        <span>工作台</span>
      </template>
      
      <el-upload
        :auto-upload="true"
        :show-file-list="true"
        :on-change="handleFileChange"
        drag
      >
        <el-icon class="el-icon--upload"><upload-filled /></el-icon>
        <div class="el-upload__text">
          将基线文档拖到此处，或<em>点击上传</em>
        </div>
      </el-upload>
    </el-card>

    <el-card style="margin-top: 20px">
      <template #header>
        <span>解析状态</span>
      </template>
      
      <el-progress
        v-if="parseStatus"
        :percentage="parseStatus.progress"
        :status="parseStatus.status === 'completed' ? 'success' : parseStatus.status === 'failed' ? 'exception' : undefined"
        :format="() => parseStatus.message"
      />
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { UploadFilled } from '@element-plus/icons-vue'
import { uploadTemplate, getTemplateStatus } from '@/api/templates'
import type { UploadFile } from 'element-plus'

const parseStatus = ref<{ progress: number; status: string; message: string } | null>(null)

const handleFileChange = async (file: UploadFile) => {
  try {
    await uploadTemplate(file.raw as File)
    parseStatus.value = { progress: 0, status: 'parsing', message: '开始解析...' }
    pollStatus()
  } catch (error) {
    parseStatus.value = { progress: 0, status: 'failed', message: '上传失败' }
  }
}

const pollStatus = () => {
  // TODO: 轮询解析状态
}
</script>