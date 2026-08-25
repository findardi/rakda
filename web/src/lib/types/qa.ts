export type QuestionStatus = 'waiting' | 'answered' | 'closed';

export interface QuestionListItem {
	id: string;
	number: number;
	subject: string;
	status: QuestionStatus;
	group_id?: string;
	group_name: string;
	author_id: string;
	author_name: string;
	reply_count: number;
	created_at: string;
}

export interface QaListData {
	items: QuestionListItem[];
	next_cursor: string;
	question_count: number;
	faq_count: number;
	qa_enabled: boolean;
	question_limit?: number;
	quota_remaining?: number;
	waiting_count?: number;
}

export interface QaQuery {
	limit?: number;
	cursor?: string;
	status?: string;
	group_id?: string;
}

export interface QaFilters {
	status: string;
	group_id: string;
}

export interface QaReference {
	id: string;
	name: string;
	folder_id?: string;
}

export interface QaReply {
	id: string;
	author_id: string;
	author_name: string;
	author_role: string;
	body: string;
	created_at: string;
}

export interface QaThread {
	id: string;
	number: number;
	subject: string;
	body: string;
	status: QuestionStatus;
	group_id?: string;
	group_name: string;
	author_id: string;
	author_name: string;
	created_at: string;
	document_ref?: QaReference;
	folder_ref?: QaReference;
	replies: QaReply[];
}

export interface QaReplyResult {
	reply: QaReply;
	question_status: QuestionStatus;
}

export interface QaFaqItem {
	id: string;
	question_text: string;
	answer_text: string;
	created_at: string;
}

export interface QaWaitingCount {
	waiting_count: number;
}

export interface CreateQuestionPayload {
	subject: string;
	body: string;
	document_id?: string;
	folder_id?: string;
}

export interface CreateFaqPayload {
	question_text: string;
	answer_text: string;
	source_question_id?: string;
}
