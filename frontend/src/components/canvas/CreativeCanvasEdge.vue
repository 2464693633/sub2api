<template>
  <BaseEdge :id="id" :path="route.path" :marker-end="markerEnd" :style="{ stroke: selected ? '#0f766e' : '#94a3b8', strokeWidth: selected ? 2.5 : 1.5, strokeLinejoin: 'round' }" :interaction-width="20" />
  <EdgeLabelRenderer v-if="label"><span class="connection-label" :style="{ transform: `translate(-50%, -50%) translate(${route.label.x}px, ${route.label.y}px)` }">{{ label }}</span></EdgeLabelRenderer>
</template>
<script setup lang="ts">
import { computed } from 'vue'
import { BaseEdge, EdgeLabelRenderer, useVueFlow, type EdgeProps } from '@vue-flow/core'
import { routeCanvasConnection } from './creativeCanvasRouting'
const props = defineProps<EdgeProps>()
const { nodes } = useVueFlow()
const route = computed(() => routeCanvasConnection({ x: props.sourceX, y: props.sourceY }, { x: props.targetX, y: props.targetY }, props.source, props.target, nodes.value.map(node => ({ id: node.id, x: node.position.x, y: node.position.y, width: node.dimensions.width || 286, height: node.dimensions.height || (node.type === 'text' ? 260 : 370) }))))
</script>
<style scoped>
.connection-label { position:absolute;pointer-events:none;border-radius:4px;padding:2px 5px;background:var(--cs-bg,#fff);color:var(--cs-muted,#64748b);font-size:10px; }
</style>
