import * as React from 'react';
import { Switch as SwitchPrimitive } from '@base-ui/react/switch';
import { cva, type VariantProps } from 'class-variance-authority';
import { BellRing } from 'lucide-react';
import { cn } from '@classding/ui/lib/utils';

// The classroom client keeps its own compact controls (styled by ui.css)
// rather than the shadcn set the web workspaces use.

const buttonVariants = cva('cd-button', {
  variants: {
    variant: {
      default: 'cd-button-primary',
      secondary: 'cd-button-secondary',
      ghost: 'cd-button-ghost',
      outline: 'cd-button-outline',
      destructive: 'cd-button-destructive',
    },
    size: { default: '', sm: 'cd-button-sm', icon: 'cd-button-icon' },
  },
  defaultVariants: { variant: 'default', size: 'default' },
});
export type ButtonProps = React.ButtonHTMLAttributes<HTMLButtonElement> & VariantProps<typeof buttonVariants>;
export const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, type = 'button', ...props }, ref) => (
    <button ref={ref} type={type} className={cn(buttonVariants({ variant, size }), className)} {...props} />
  ),
);
Button.displayName = 'Button';
export const Input = React.forwardRef<HTMLInputElement, React.InputHTMLAttributes<HTMLInputElement>>(
  ({ className, ...props }, ref) => <input ref={ref} className={cn('cd-input', className)} {...props} />,
);
Input.displayName = 'Input';
export function Card({ className, ...props }: React.HTMLAttributes<HTMLDivElement>) {
  return <div className={cn('cd-card', className)} {...props} />;
}
export function Badge({
  className,
  variant = 'default',
  ...props
}: React.HTMLAttributes<HTMLSpanElement> & {
  variant?: 'default' | 'secondary' | 'outline' | 'success' | 'warning' | 'destructive';
}) {
  return <span className={cn('cd-badge', `cd-badge-${variant}`, className)} {...props} />;
}
export function Switch({
  checked,
  onCheckedChange,
  className,
  ...props
}: {
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  className?: string;
  id?: string;
  'aria-label'?: string;
  disabled?: boolean;
}) {
  return (
    <SwitchPrimitive.Root
      checked={checked}
      onCheckedChange={onCheckedChange}
      className={cn('cd-switch', className)}
      {...props}
    >
      <SwitchPrimitive.Thumb className="cd-switch-thumb" />
    </SwitchPrimitive.Root>
  );
}
export function Brand({ compact = false, className }: { compact?: boolean; className?: string }) {
  return (
    <div className={cn('cd-brand', className)}>
      <span className="cd-brand-mark">
        <BellRing size={23} strokeWidth={2.1} />
        <i />
      </span>
      {!compact && (
        <span>
          <strong>
            课堂叮咚<span className="cd-brand-dot">.</span>
          </strong>
          <small>CLASSDING</small>
        </span>
      )}
    </div>
  );
}
