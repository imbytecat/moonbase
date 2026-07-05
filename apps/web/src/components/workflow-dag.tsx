import dagre from '@dagrejs/dagre'
import type { WorkflowStep } from '@moonbase/api-client'
import {
  Background,
  Controls,
  Handle,
  type Node,
  type NodeProps,
  Position,
  ReactFlow,
} from '@xyflow/react'
import { Tag, Typography } from 'antd'
import { useMemo } from 'react'
import { m } from '#paraglide/messages.js'

import '@xyflow/react/dist/style.css'

// Renders one run's execution trace as a DAG: START → step1 → step2 … in
// checkpoint order, plus dashed edges to child workflows a step spawned.
// This is the runtime truth from the durable checkpoint table — not an
// editable flow definition.

const NODE_WIDTH = 220
const NODE_HEIGHT = 72

type StepNodeData = {
  label: string
  status: 'success' | 'error' | 'start'
  duration?: string
  error?: string
  [key: string]: unknown
}

type FlowNode = Node<StepNodeData>

function StepNode({ data }: NodeProps<FlowNode>) {
  return (
    <div
      className={`w-52 rounded-lg border bg-(--ant-color-bg-container) px-3 py-2 shadow-sm ${
        data.status === 'error'
          ? 'border-(--ant-color-error)'
          : data.status === 'start'
            ? 'border-(--ant-color-primary)'
            : 'border-(--ant-color-border)'
      }`}
    >
      <Handle type="target" position={Position.Top} className="!bg-(--ant-color-text-quaternary)" />
      <div className="flex items-center justify-between gap-2">
        <Typography.Text strong className="truncate text-xs">
          {data.label}
        </Typography.Text>
        {data.status === 'error' ? (
          <Tag color="red" className="!me-0">
            {m.workflows_stepFailed()}
          </Tag>
        ) : null}
      </div>
      {data.duration ? (
        <div className="mt-0.5 text-[11px] text-(--ant-color-text-tertiary)">{data.duration}</div>
      ) : null}
      {data.error ? (
        <div className="mt-0.5 truncate text-[11px] text-(--ant-color-error)">{data.error}</div>
      ) : null}
      <Handle
        type="source"
        position={Position.Bottom}
        className="!bg-(--ant-color-text-quaternary)"
      />
    </div>
  )
}

const nodeTypes = { step: StepNode }

function layout(nodes: FlowNode[], edges: { id: string; source: string; target: string }[]) {
  const g = new dagre.graphlib.Graph()
  g.setGraph({ rankdir: 'TB', nodesep: 40, ranksep: 48 })
  g.setDefaultEdgeLabel(() => ({}))
  for (const node of nodes) {
    g.setNode(node.id, { width: NODE_WIDTH, height: NODE_HEIGHT })
  }
  for (const edge of edges) {
    g.setEdge(edge.source, edge.target)
  }
  dagre.layout(g)
  return nodes.map((node) => {
    const pos = g.node(node.id)
    return {
      ...node,
      position: { x: pos.x - NODE_WIDTH / 2, y: pos.y - NODE_HEIGHT / 2 },
    }
  })
}

function stepDuration(step: WorkflowStep): string | undefined {
  if (!step.startedAt || !step.completedAt) return undefined
  const ms = Number(step.completedAt.seconds - step.startedAt.seconds) * 1000
  if (ms < 1000) return '<1s'
  return `${(ms / 1000).toFixed(0)}s`
}

export function WorkflowDag({
  workflowName,
  steps,
}: {
  workflowName: string
  steps: WorkflowStep[]
}) {
  const { nodes, edges } = useMemo(() => {
    const flowNodes: FlowNode[] = [
      {
        id: '__start',
        type: 'step',
        position: { x: 0, y: 0 },
        data: { label: workflowName, status: 'start' },
      },
    ]
    const flowEdges: {
      id: string
      source: string
      target: string
      animated?: boolean
      style?: React.CSSProperties
    }[] = []

    let prev = '__start'
    for (const step of steps) {
      const id = `step-${step.stepId}`
      flowNodes.push({
        id,
        type: 'step',
        position: { x: 0, y: 0 },
        data: {
          label: step.stepName || `#${step.stepId}`,
          status: step.error ? 'error' : 'success',
          duration: stepDuration(step),
          error: step.error || undefined,
        },
      })
      flowEdges.push({ id: `${prev}->${id}`, source: prev, target: id })
      if (step.childWorkflowId) {
        const childId = `child-${step.stepId}`
        flowNodes.push({
          id: childId,
          type: 'step',
          position: { x: 0, y: 0 },
          data: { label: `↳ ${step.childWorkflowId.slice(0, 8)}…`, status: 'start' },
        })
        flowEdges.push({
          id: `${id}->${childId}`,
          source: id,
          target: childId,
          animated: true,
          style: { strokeDasharray: '4 4' },
        })
      }
      prev = id
    }
    return { nodes: layout(flowNodes, flowEdges), edges: flowEdges }
  }, [workflowName, steps])

  return (
    <div className="h-96 rounded-lg border border-(--ant-color-border)">
      <ReactFlow
        nodes={nodes}
        edges={edges}
        nodeTypes={nodeTypes}
        fitView
        proOptions={{ hideAttribution: true }}
        nodesDraggable={false}
        nodesConnectable={false}
        elementsSelectable={false}
      >
        <Background />
        <Controls showInteractive={false} />
      </ReactFlow>
    </div>
  )
}
