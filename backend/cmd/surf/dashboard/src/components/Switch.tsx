interface SwitchProps {
  checked: boolean;
  /** Id of the element that names this switch. */
  labelledBy: string;
  disabled?: boolean;
  onChange: (checked: boolean) => void;
}

export function Switch({ checked, labelledBy, disabled, onChange }: SwitchProps) {
  return (
    <button
      type="button"
      role="switch"
      class="switch"
      aria-checked={checked}
      aria-labelledby={labelledBy}
      disabled={disabled}
      onClick={() => onChange(!checked)}
    >
      <span class="switch-knob" aria-hidden="true" />
    </button>
  );
}
