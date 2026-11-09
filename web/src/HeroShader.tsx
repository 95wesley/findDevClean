// Full-page background effect. Loaded lazily from App so the WebGPU runtime
// (~1.3 MB) only downloads when the effect is turned on.
import { Aurora, Shader } from "shaders/react";

export default function HeroShader({ onUnavailable }: { onUnavailable: () => void }) {
  return (
    <div className="fx-bg" aria-hidden="true">
      <Shader className="fx-canvas" onUnavailable={onUnavailable}>
        <Aurora />
      </Shader>
    </div>
  );
}
