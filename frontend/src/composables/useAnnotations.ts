import { ref, watchEffect } from 'vue'
import {
  fetchAnnotations,
  type Annotation,
  type AnnotationTargetType,
} from '../api/annotations'

// useAnnotations loads annotations for a target. targetId is a getter so the
// caller can pass a reactive route param; when it returns undefined the call is
// skipped-by-passing-no-targetId (used by pages that fetch all-of-type).
export function useAnnotations(
  targetType: AnnotationTargetType,
  targetId: () => string | undefined,
) {
  const annotations = ref<Annotation[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)

  async function load(): Promise<void> {
    const id = targetId()
    loading.value = true
    error.value = null
    try {
      annotations.value = await fetchAnnotations(targetType, id ? { targetId: id } : {})
    } catch (e) {
      error.value = (e as Error).message
    } finally {
      loading.value = false
    }
  }

  watchEffect(() => {
    void targetId() // track reactive dependency
    void load()
  })

  return { annotations, loading, error, reload: load }
}
