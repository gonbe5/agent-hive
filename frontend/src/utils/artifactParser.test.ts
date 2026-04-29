import { describe, it, expect } from 'vitest';
import {
  parseMessageContent,
  parseMessageContentWithSkeleton,
  hasOpenArtifact,
} from './artifactParser';

describe('artifactParser', () => {
  // ==================== parseMessageContent ====================

  describe('parseMessageContent', () => {
    it('无标签 → 单文本片段', () => {
      expect(parseMessageContent('plain text')).toEqual([
        { type: 'text', content: 'plain text' },
      ]);
    });

    it('单 artifact，中间文本', () => {
      const result = parseMessageContent('head <artifact type="markdown" title="T">content</artifact> tail');
      expect(result).toHaveLength(3);
      expect(result[0]).toEqual({ type: 'text', content: 'head ' });
      expect(result[1]).toEqual({
        type: 'artifact',
        content: 'content',
        artifactType: 'markdown',
        title: 'T',
        isLoading: false,
      });
      expect(result[2]).toEqual({ type: 'text', content: ' tail' });
    });

    it('多个 artifact，之间有文本', () => {
      const result = parseMessageContent(
        '<artifact type="markdown" title="A">a</artifact> mid <artifact type="html" title="B">b</artifact>'
      );
      expect(result).toHaveLength(3);
      expect(result[0].type).toBe('artifact');
      expect(result[0].title).toBe('A');
      expect(result[1].type).toBe('text');
      expect(result[1].content).toBe(' mid ');
      expect(result[2].type).toBe('artifact');
      expect(result[2].title).toBe('B');
    });

    it('属性顺序任意', () => {
      const result = parseMessageContent(
        '<artifact title="T" type="html">...</artifact>'
      );
      expect(result[0].artifactType).toBe('html');
      expect(result[0].title).toBe('T');
    });

    it('language 属性', () => {
      const result = parseMessageContent(
        '<artifact type="code" language="python" title="T">code</artifact>'
      );
      expect(result[0].artifactType).toBe('code');
      expect(result[0].language).toBe('python');
    });

    it('title 为空 → fallback 到文档', () => {
      const result = parseMessageContent('<artifact type="markdown" title="">content</artifact>');
      expect(result[0].title).toBe('文档');
    });

    it('type 无效 → fallback 到 markdown', () => {
      const result = parseMessageContent('<artifact type="unknown" title="T">content</artifact>');
      expect(result[0].artifactType).toBe('markdown');
    });

    it('负面：subtitle 不应误匹配', () => {
      const result = parseMessageContent(
        '<artifact subtitle="wrong" type="markdown" title="T">content</artifact>'
      );
      expect(result[0].title).toBe('T');
      expect(result[0].artifactType).toBe('markdown');
    });

    it('负面：data-type 不应误匹配', () => {
      const result = parseMessageContent(
        '<artifact data-type="wrong" type="markdown" title="T">content</artifact>'
      );
      expect(result[0].artifactType).toBe('markdown');
    });

    it('内容保留内部空行', () => {
      const result = parseMessageContent(
        '<artifact type="markdown" title="T">\n\n# H\n\npara\n\n</artifact>'
      );
      expect(result[0].content).toBe('# H\n\npara');
    });

    it('负面：嵌套标签被截断（已知限制）', () => {
      const result = parseMessageContent(
        '<artifact type="markdown" title="Outer"><artifact type="html" title="Inner">nested</artifact></artifact>'
      );
      // 只匹配到第一个 </artifact>，外层 </artifact> 成为尾部文本
      expect(result).toHaveLength(2);
      expect(result[0].type).toBe('artifact');
      expect(result[1].type).toBe('text');
      expect(result[1].content).toBe('</artifact>');
    });

    it('负面：内容含字面量 </artifact> 字符串（已知限制）', () => {
      const result = parseMessageContent(
        '<artifact type="markdown" title="T">Before </artifact> after</artifact>'
      );
      expect(result).toHaveLength(2);
      expect(result[0].content).toBe('Before ');
      expect(result[1].content).toContain('after</artifact');
    });
  });

  // ==================== hasOpenArtifact ====================

  describe('hasOpenArtifact', () => {
    it('未闭合 → true', () => {
      expect(hasOpenArtifact('text <artifact type="markdown" title="T">partial')).toBe(true);
    });

    it('已闭合 → false', () => {
      expect(hasOpenArtifact('<artifact type="markdown" title="T">done</artifact>')).toBe(false);
    });

    it('空字符串 → false', () => {
      expect(hasOpenArtifact('')).toBe(false);
    });

    it('只有开标签 → true', () => {
      expect(hasOpenArtifact('<artifact')).toBe(true);
    });
  });

  // ==================== parseMessageContentWithSkeleton ====================

  describe('parseMessageContentWithSkeleton', () => {
    it('未闭合 → 骨架段', () => {
      const result = parseMessageContentWithSkeleton('<artifact type="markdown" title="T">partial');
      expect(result).toHaveLength(1);
      expect(result[0].type).toBe('artifact');
      expect(result[0].isLoading).toBe(true);
      expect(result[0].title).toBe('T');
    });

    it('已闭合 + 未闭合 → 混合', () => {
      const result = parseMessageContentWithSkeleton(
        '<artifact type="markdown" title="A">done</artifact> text <artifact type="html" title="B">partial'
      );
      expect(result).toHaveLength(3);
      expect(result[0].type).toBe('artifact');
      expect(result[0].isLoading).toBe(false);
      expect(result[0].title).toBe('A');
      expect(result[1].type).toBe('text');
      expect(result[1].content).toBe(' text ');
      expect(result[2].type).toBe('artifact');
      expect(result[2].isLoading).toBe(true);
      expect(result[2].title).toBe('B');
    });

    it('属性顺序任意（骨架）', () => {
      const result = parseMessageContentWithSkeleton(
        '<artifact title="T" type="code" language="go">partial'
      );
      expect(result[0].type).toBe('artifact');
      expect(result[0].isLoading).toBe(true);
      expect(result[0].title).toBe('T');
      expect(result[0].artifactType).toBe('code');
      expect(result[0].language).toBe('go');
    });

    // 流式分片场景
    it('流式分片：<arti → 文本，不是骨架', () => {
      const result = parseMessageContentWithSkeleton('<arti');
      expect(result).toHaveLength(1);
      expect(result[0].type).toBe('text');
      expect(result[0].content).toBe('<arti');
    });

    it('流式分片：<artifact → 文本，不是骨架', () => {
      const result = parseMessageContentWithSkeleton('<artifact');
      expect(result).toHaveLength(1);
      expect(result[0].type).toBe('text');
      expect(result[0].content).toBe('<artifact');
    });

    it('流式分片：<artifact type="mark → 文本（> 未到），不是骨架', () => {
      const result = parseMessageContentWithSkeleton('<artifact type="mark');
      expect(result).toHaveLength(1);
      expect(result[0].type).toBe('text');
      expect(result[0].content).toBe('<artifact type="mark');
    });

    it('完整开标签，无闭合 → 骨架', () => {
      const result = parseMessageContentWithSkeleton('<artifact type="markdown" title="T">');
      expect(result).toHaveLength(1);
      expect(result[0].type).toBe('artifact');
      expect(result[0].isLoading).toBe(true);
      expect(result[0].title).toBe('T');
    });

    it('部分闭合标签（如 </arti） → 骨架（不匹配 >）', () => {
      const result = parseMessageContentWithSkeleton('<artifact type="markdown" title="T">content</arti');
      expect(result).toHaveLength(1);
      expect(result[0].type).toBe('artifact');
      expect(result[0].isLoading).toBe(true);
      expect(result[0].title).toBe('T');
    });

    it('纯文本，无标签', () => {
      const result = parseMessageContentWithSkeleton('plain text');
      expect(result).toHaveLength(1);
      expect(result[0].type).toBe('text');
      expect(result[0].content).toBe('plain text');
    });
  });
});
