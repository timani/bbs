package format_test

import (
	"encoding/json"

	"github.com/gogo/protobuf/proto"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pivotal-golang/lager"
	"github.com/pivotal-golang/lager/lagertest"

	"github.com/cloudfoundry-incubator/bbs/encryption"
	"github.com/cloudfoundry-incubator/bbs/encryption/encryptionfakes"
	"github.com/cloudfoundry-incubator/bbs/format"
	"github.com/cloudfoundry-incubator/bbs/models"
	"github.com/cloudfoundry-incubator/bbs/models/test/model_helpers"
)

var _ = Describe("Format", func() {
	var (
		serializer format.Serializer
		cryptor    *encryptionfakes.FakeCryptor
		encoder    format.Encoder
		logger     lager.Logger
		task       *models.Task
	)

	BeforeEach(func() {
		task = model_helpers.NewValidTask("a-guid")
		logger = lagertest.NewTestLogger("test")
		cryptor = &encryptionfakes.FakeCryptor{}
		cryptor.EncryptStub = func(plaintext []byte) (encryption.Encrypted, error) {
			nonce := [12]byte{}
			return encryption.Encrypted{
				KeyLabel:   "label",
				Nonce:      nonce[:],
				CipherText: plaintext,
			}, nil
		}
		cryptor.DecryptStub = func(ciphered encryption.Encrypted) ([]byte, error) {
			return ciphered.CipherText, nil
		}
		encoder = format.NewEncoder(cryptor)
		serializer = format.NewSerializer(cryptor)
	})

	Describe("Marshal", func() {
		Describe("LEGACY_FORMATTING", func() {
			It("marshals the data as-is without an envelope", func() {
				jsonEncodedTask, err := json.Marshal(task)
				Expect(err).NotTo(HaveOccurred())

				encoded, err := serializer.Marshal(logger, format.LEGACY_FORMATTING, task)
				Expect(err).NotTo(HaveOccurred())
				Expect(encoded).To(MatchJSON(jsonEncodedTask))
			})
		})

		Describe("FORMATTED_JSON", func() {
			It("marshals the data as json with an UNENCODED envelope", func() {
				jsonEncodedTask, err := json.Marshal(task)
				Expect(err).NotTo(HaveOccurred())

				encoded, err := serializer.Marshal(logger, format.FORMATTED_JSON, task)
				Expect(err).NotTo(HaveOccurred())

				unencoded, err := encoder.Decode(encoded)
				Expect(err).NotTo(HaveOccurred())

				Expect(unencoded[0]).To(BeEquivalentTo(format.JSON))
				Expect(unencoded[2:]).To(MatchJSON(jsonEncodedTask))
			})
		})

		Describe("ENCODED_PROTO", func() {
			It("marshals the data as protobuf with an base64 encoded envelope", func() {
				protoEncodedTask, err := proto.Marshal(task)
				Expect(err).NotTo(HaveOccurred())

				encoded, err := serializer.Marshal(logger, format.ENCODED_PROTO, task)
				Expect(err).NotTo(HaveOccurred())

				unencoded, err := encoder.Decode(encoded)
				Expect(err).NotTo(HaveOccurred())

				Expect(unencoded[0]).To(BeEquivalentTo(format.PROTO))
				Expect(unencoded[2:]).To(Equal(protoEncodedTask))
			})
		})

		Describe("ENCRYPTED_PROTO", func() {
			It("marshals the data as protobuf with an base64 encoded ciphertext envelope", func() {
				protoEncodedTask, err := proto.Marshal(task)
				Expect(err).NotTo(HaveOccurred())

				encoded, err := serializer.Marshal(logger, format.ENCRYPTED_PROTO, task)
				Expect(err).NotTo(HaveOccurred())

				unencoded, err := encoder.Decode(encoded)
				Expect(err).NotTo(HaveOccurred())

				Expect(unencoded[0]).To(BeEquivalentTo(format.PROTO))
				Expect(unencoded[2:]).To(Equal(protoEncodedTask))
			})
		})
	})

	Describe("Unmarshal", func() {
		Describe("LEGACY_FORMATTING", func() {
			It("unmarshals the JSON data as-is without an envelope", func() {
				payload, err := serializer.Marshal(logger, format.LEGACY_FORMATTING, task)
				Expect(err).NotTo(HaveOccurred())

				var decodedTask models.Task
				err = serializer.Unmarshal(logger, payload, &decodedTask)
				Expect(err).NotTo(HaveOccurred())
				Expect(*task).To(Equal(decodedTask))
			})
		})

		Describe("FORMATTED_JSON", func() {
			It("unmarshals the JSON data from an UNENCODED envelope", func() {
				payload, err := serializer.Marshal(logger, format.FORMATTED_JSON, task)
				Expect(err).NotTo(HaveOccurred())

				var decodedTask models.Task
				err = serializer.Unmarshal(logger, payload, &decodedTask)
				Expect(err).NotTo(HaveOccurred())
				Expect(*task).To(Equal(decodedTask))
			})
		})

		Describe("ENCODED_PROTO", func() {
			It("unmarshals the protobuf data from a base64 envelope", func() {
				payload, err := serializer.Marshal(logger, format.ENCODED_PROTO, task)
				Expect(err).NotTo(HaveOccurred())

				var decodedTask models.Task
				err = serializer.Unmarshal(logger, payload, &decodedTask)
				Expect(err).NotTo(HaveOccurred())
				Expect(*task).To(Equal(decodedTask))
			})
		})

		Describe("ENCRYPTED_PROTO", func() {
			It("unmarshals the protobuf data from a base64 encoded ciphertext envelope", func() {
				payload, err := serializer.Marshal(logger, format.ENCRYPTED_PROTO, task)
				Expect(err).NotTo(HaveOccurred())

				var decodedTask models.Task
				err = serializer.Unmarshal(logger, payload, &decodedTask)
				Expect(err).NotTo(HaveOccurred())
				Expect(*task).To(Equal(decodedTask))
			})
		})
	})
})
