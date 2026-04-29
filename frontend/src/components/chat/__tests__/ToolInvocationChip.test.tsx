import { describe, it, expect, vi } from 'vitest';
import { render, screen } from '@testing-library/react';
import { ToolInvocationChip } from '../ToolInvocationChip';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => (key === 'tools.invoked' ? 'Called tool' : key),
  }),
}));

vi.mock('../../../store/chat', () => ({
  useChatStore: (selector: (s: unknown) => unknown) => selector({ toolCallStatuses: {} }),
}));

vi.mock('../../../utils/toolName', () => ({
  getToolDisplayName: (name: string) => (name === 'bash_exec' ? 'Shell' : name),
}));

describe('ToolInvocationChip', () => {
  it('renders success state with Settings icon and aria-label', () => {
    render(<ToolInvocationChip name="bash_exec" status="success" />);
    const pill = screen.getByRole('status');
    expect(pill).toHaveAttribute('aria-label', 'Called tool: Shell');
    expect(pill.textContent).toContain('Shell');
    expect(pill.querySelector('.animate-spin')).toBeNull();
  });

  it('renders running state with spinner', () => {
    const { container } = render(
      <ToolInvocationChip name="bash_exec" status="running" />
    );
    expect(container.querySelector('.animate-spin')).not.toBeNull();
  });

  it('renders error state with danger color class', () => {
    const { container } = render(
      <ToolInvocationChip name="bash_exec" status="error" />
    );
    const icon = container.querySelector('svg');
    expect(icon?.getAttribute('class') ?? '').toMatch(/text-\[var\(--danger\)\]/);
  });

  it('has no expand button (chip is display-only)', () => {
    render(<ToolInvocationChip name="bash_exec" status="success" />);
    expect(screen.queryByRole('button')).toBeNull();
  });

  it('uses getToolDisplayName for label', () => {
    render(<ToolInvocationChip name="bash_exec" status="success" />);
    expect(screen.getByRole('status').textContent).toContain('Shell');
  });
});
