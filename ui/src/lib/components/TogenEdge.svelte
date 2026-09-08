<script lang="ts">
  import { BaseEdge, EdgeLabel, useViewport, type EdgeProps } from '@xyflow/svelte';

  import { chipAt, edgePath, headPath, strokeFor, type Band, type Texture } from '../edges.ts';

  let { sourceX, sourceY, targetX, targetY, data, label }: EdgeProps = $props();

  const viewport = useViewport();
  const edge = $derived(
    data as unknown as {
      texture: Texture;
      band: Band;
      lane: number;
      pair: number;
      source: number;
      target: number;
      chip: number;
    },
  );

  // The bundle offset moves both ends together, the fan only the end it belongs to.
  const from = $derived(sourceY + edge.pair + edge.source);
  const to = $derived(targetY + edge.pair + edge.target);

  const stroke = $derived(strokeFor(edge.texture, edge.band, viewport.current.zoom));
  const path = $derived(
    edgePath({
      sourceX,
      sourceY: from,
      targetX,
      targetY: to,
      lane: edge.lane,
      width: stroke.width,
    }),
  );
  const chipX = $derived(chipAt(sourceX, targetX, stroke.width, edge.chip));
  const style = $derived(
    [
      `stroke-width: ${stroke.width}px`,
      `stroke-linecap: ${stroke.cap}`,
      stroke.dash === null ? '' : `stroke-dasharray: ${stroke.dash}`,
    ]
      .filter((part) => part !== '')
      .join('; '),
  );
</script>

<!-- The casing stays solid under a broken stroke, so a dash or a dot keeps its
     priority at a crossing instead of dissolving into the line beneath. -->
<path class="togen-edge-casing" d={path} style="stroke-width: {stroke.width + 5}px" />
<BaseEdge {path} {style} />
<path class="togen-edge-head" d={headPath(targetX, to, stroke.width)} />
{#if label !== undefined && label !== '' && chipX !== null}
  <EdgeLabel x={chipX} y={to}>
    <span class="togen-chip">{label}</span>
  </EdgeLabel>
{/if}
